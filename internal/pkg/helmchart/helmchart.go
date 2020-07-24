package helmchart

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/mikefarah/yq/v3/pkg/yqlib"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

// RenderTarget contains all informations needed for rendering a chart
type RenderTarget struct {
	RepoConfig     repo.Entry
	ChartReference string
	ChartVersion   string
	Values         map[string]interface{}
	Verify         bool
}

// Renderer for helm charts
type Renderer struct {
	repoCache   string
	repoFile    string
	downloadDir string
	settings    *cli.EnvSettings
	getters     getter.Providers
}

// NewRenderer creates a default Renderer instance.
// Panics if the required cache directories are non-existant and can not be created.
func NewRenderer() Renderer {
	result := Renderer{}
	result.repoCache = envOrDefault("HELM_REPO_CACHE_DIR", "/tmp/saveomat")
	result.repoFile = envOrDefault("HELM_REPO_CONFIG_FILE", "/tmp/saveomat/helm.yaml")
	result.downloadDir = envOrDefault("HELM_DOWNLOAD_DIR", "/tmp/saveomat")
	result.settings = &cli.EnvSettings{
		RepositoryConfig: result.repoFile,
		RepositoryCache:  result.repoCache,
	}
	result.getters = getter.All(result.settings)

	err := os.MkdirAll(result.repoCache, 0755)
	if err != nil && !os.IsExist(err) {
		panic(err)
	}
	err = os.MkdirAll(result.downloadDir, 0755)
	if err != nil && !os.IsExist(err) {
		panic(err)
	}

	return result
}

// ensureRepo adds a chart repository if it does not yet exist in the config
func (r *Renderer) ensureRepo(cfg repo.Entry) error {
	// TODO check whether it already exist (update instead)

	// The following code is adapted from https://github.com/helm/helm/blob/master/cmd/helm/repo_add.go
	fileLock := flock.New(strings.Replace(r.repoFile, filepath.Ext(r.repoFile), ".lock", 1))
	lockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	locked, err := fileLock.TryLockContext(lockCtx, time.Second)
	if err == nil && locked {
		defer fileLock.Unlock()
	}
	if err != nil {
		return err
	}

	b, err := ioutil.ReadFile(r.repoFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var f repo.File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return err
	}

	repo, err := repo.NewChartRepository(&cfg, r.getters)
	if err != nil {
		return err
	}
	if r.repoCache != "" {
		repo.CachePath = r.repoCache
	}

	if _, err := repo.DownloadIndexFile(); err != nil {
		return errors.Wrapf(err, "looks like %q is not a valid chart repository or cannot be reached", cfg.URL)
	}

	f.Update(&cfg)
	return f.WriteFile(r.repoFile, 0644)
}

// Render the given chart, returning the resulting kubernetes manifest
func (r *Renderer) Render(target RenderTarget) (string, error) {
	if err := r.ensureRepo(target.RepoConfig); err != nil {
		return "", err
	}

	// The following code is adapted from https://github.com/helm/helm/blob/master/cmd/helm/install.go
	client := action.NewInstall(&action.Configuration{})
	client.ClientOnly = true
	client.DryRun = true
	client.Version = target.ChartVersion
	client.Verify = target.Verify
	name, chart, err := client.NameAndChart([]string{"saveomat", target.ChartReference})
	if err != nil {
		return "", err
	}
	client.ReleaseName = name

	cp, err := client.ChartPathOptions.LocateChart(chart, r.settings)
	if err != nil {
		return "", err
	}

	// Check chart dependencies to make sure all are present in /charts
	chartRequested, err := loader.Load(cp)
	if err != nil {
		return "", err
	}
	if req := chartRequested.Metadata.Dependencies; req != nil {
		// If CheckDependencies returns an error, we have unfulfilled dependencies.
		// As of Helm 2.4.0, this is treated as a stopping condition:
		// https://github.com/helm/helm/issues/2209
		if err := action.CheckDependencies(chartRequested, req); err != nil {
			if client.DependencyUpdate {
				man := &downloader.Manager{
					Out:              os.Stdout,
					ChartPath:        cp,
					Keyring:          client.ChartPathOptions.Keyring,
					SkipUpdate:       false,
					Getters:          r.getters,
					RepositoryConfig: r.settings.RepositoryConfig,
					RepositoryCache:  r.settings.RepositoryCache,
					Debug:            r.settings.Debug,
				}
				if err := man.Update(); err != nil {
					return "", err
				}
				// Reload the chart with the updated Chart.lock file.
				if chartRequested, err = loader.Load(cp); err != nil {
					return "", errors.Wrap(err, "failed reloading chart after repo update")
				}
			} else {
				return "", err
			}
		}
	}
	release, err := client.Run(chartRequested, target.Values)
	if err != nil {
		return "", err
	}
	return release.Manifest, nil
}

// FindImagesInManifest searches for container images inside a kubernetes manifest.
// Any key named "image" is considered to be a image name (the manifest is not deeply parsed).
// The provided manifest can contain multiple yaml nodes.
func FindImagesInManifest(yml string) ([]string, error) {
	var result []string
	r := strings.NewReader(yml)
	defer ioutil.ReadAll(r)

	var node yaml.Node
	var readErr error
	for decoder := yaml.NewDecoder(r); readErr != io.EOF; readErr = decoder.Decode(&node) {
		if readErr != nil {
			return nil, readErr
		}
		images, err := findImagesInNode(node)
		if err != nil {
			return nil, err
		}
		result = append(result, images...)
	}
	return result, nil
}

func findImagesInNode(node yaml.Node) ([]string, error) {
	var result []string
	nodes, err := yqlib.NewYqLib().Get(&node, "**.image", false)
	if err != nil {
		return nil, err
	}
	for _, match := range nodes {
		result = append(result, fmt.Sprintf("%s", match.Node.Value))
	}
	return result, nil
}

func envOrDefault(key, dflt string) string {
	if e := os.Getenv(key); e != "" {
		return e
	}
	return dflt
}
