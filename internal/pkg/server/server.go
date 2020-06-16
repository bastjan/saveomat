package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bastjan/saveomat/internal/pkg/auth"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gofrs/flock"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
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

var (
	repoCache   = envOrDefault("HELM_REPO_CACHE_DIR", "/tmp/saveomat")
	repoFile    = envOrDefault("HELM_REPO_CONFIG_FILE", "/tmp/saveomat/helm.yaml")
	downloadDir = envOrDefault("HELM_DOWNLOAD_DIR", "/tmp/saveomat")
	settings    = &cli.EnvSettings{
		RepositoryConfig: repoFile,
		RepositoryCache:  repoCache,
	}
	getters = getter.All(settings)
)

type ServerOpts struct {
	BaseURL      string
	DockerClient client.ImageAPIClient
}

type Server struct {
	*echo.Echo
	DockerClient client.ImageAPIClient
}

func NewServer(opt ServerOpts) *Server {
	err := os.MkdirAll(repoCache, 0755)
	if err != nil && !os.IsExist(err) {
		panic(err)
	}
	err = os.MkdirAll(downloadDir, 0755)
	if err != nil && !os.IsExist(err) {
		panic(err)
	}

	e := echo.New()
	s := &Server{e, opt.DockerClient}
	s.Logger.SetLevel(env2Lvl("LOG_LEVEL"))

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("512K"))

	baseurl := strings.TrimSuffix(opt.BaseURL, "/")
	g := e.Group(baseurl)
	g.GET("/", func(c echo.Context) error {
		return c.HTML(http.StatusOK, indexHTML)
	})
	g.POST("/tar", s.postTar)
	g.GET("/tar", s.getTar)
	g.POST("/tar/helm/repo", s.postHelmRepo)
	g.POST("/tar/helm/chart", s.postHelmChart)

	return s
}

func (s *Server) getTar(c echo.Context) error {
	p := c.QueryParams()
	images, ok := p["image"]
	if !ok {
		return c.NoContent(http.StatusBadRequest)
	}

	return s.streamImages(c, auth.EmptyAuthenticator, normalizeImages(images))
}

func (s *Server) postTar(c echo.Context) error {
	file, err := c.FormFile("images.txt")
	if err != nil {
		return err
	}
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	authn, err := authFromFormFile(c, "config.json")
	if err != nil {
		return err
	}

	images := make([]string, 0, 5)
	sc := bufio.NewScanner(src)
	for sc.Scan() {
		images = append(images, sc.Text())
	}
	if sc.Err() != nil {
		return sc.Err()
	}

	return s.streamImages(c, authn, normalizeImages(images))
}

func (s *Server) postHelmRepo(c echo.Context) error {
	name := c.FormValue("name")
	url := c.FormValue("url")
	if name == "" {
		return fmt.Errorf("repository name not set")
	}
	if url == "" {
		return fmt.Errorf("repository URL not set")
	}

	// The following code is adapted from https://github.com/helm/helm/blob/master/cmd/helm/repo_add.go
	fileLock := flock.New(strings.Replace(repoFile, filepath.Ext(repoFile), ".lock", 1))
	lockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	locked, err := fileLock.TryLockContext(lockCtx, time.Second)
	if err == nil && locked {
		defer fileLock.Unlock()
	}
	if err != nil {
		return err
	}

	b, err := ioutil.ReadFile(repoFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var f repo.File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return err
	}

	cfg := repo.Entry{
		Name:                  name,
		URL:                   url,
		Username:              c.FormValue("username"),
		Password:              c.FormValue("password"),
		InsecureSkipTLSverify: false,
	}
	repo, err := repo.NewChartRepository(&cfg, getters)
	if err != nil {
		return err
	}
	if repoCache != "" {
		repo.CachePath = repoCache
	}

	if _, err := repo.DownloadIndexFile(); err != nil {
		return errors.Wrapf(err, "looks like %q is not a valid chart repository or cannot be reached", url)
	}

	f.Update(&cfg)
	if err := f.WriteFile(repoFile, 0644); err != nil {
		return err
	}

	c.Logger().Info(fmt.Sprintf("Repository addedd successfully: %s %s", name, url))
	return nil
}

func (s *Server) postHelmChart(c echo.Context) error {
	chrtRef := c.FormValue("chart")
	chrtVer := c.FormValue("version")
	file, err := c.FormFile("values.yaml")
	if err != nil {
		return err
	}
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	buf, err := ioutil.ReadAll(src)
	if err != nil {
		return err
	}
	values := map[string]interface{}{}
	if err := yaml.Unmarshal(buf, &values); err != nil {
		return err
	}
	var authn auth.Authenticator
	_, ok := c.QueryParams()["auth"] // Currently support only docker config.json
	if ok {
		var err error
		authn, err = authFromFormFile(c, "config.json")
		if err != nil {
			return err
		}
	} else {
		authn = auth.EmptyAuthenticator
	}
	_, verify := c.QueryParams()["verify"]
	manifest, err := renderHelmChart(chrtRef, chrtVer, values, verify)
	if err != nil {
		return err
	}
	images, err := findImagesInManifest(manifest)
	if err != nil {
		return err
	}
	normalized := normalizeImages(images)
	c.Logger().Info(fmt.Sprintf("Found %d images: %s", len(normalized), normalized))
	return s.streamImages(c, authn, normalized)
}

func renderHelmChart(reference, version string, values map[string]interface{}, verify bool) (string, error) {
	// The following code is adapted from https://github.com/helm/helm/blob/master/cmd/helm/install.go
	client := action.NewInstall(&action.Configuration{})
	client.ClientOnly = true
	client.DryRun = true
	client.Version = version
	client.Verify = verify
	name, chart, err := client.NameAndChart([]string{"saveomat", reference})
	if err != nil {
		return "", err
	}
	client.ReleaseName = name

	cp, err := client.ChartPathOptions.LocateChart(chart, settings)
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
					Getters:          getters,
					RepositoryConfig: settings.RepositoryConfig,
					RepositoryCache:  settings.RepositoryCache,
					Debug:            settings.Debug,
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
	release, err := client.Run(chartRequested, values)
	if err != nil {
		return "", err
	}
	return release.Manifest, nil
}

func findImagesInManifest(yml string) ([]string, error) {
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

func normalizeImages(images []string) []string {
	normalized := make([]string, 0, len(images))
	for _, img := range images {
		img = strings.Trim(img, " ")
		if img == "" || strings.HasPrefix(img, "#") {
			continue
		}
		normalized = append(normalized, img)
	}
	return normalized
}

func (s *Server) streamImages(c echo.Context, pullAuth auth.Authenticator, images []string) error {
	if len(images) == 0 {
		return c.NoContent(http.StatusBadRequest)
	}

	tar, err := s.pullAndSaveImages(c.Request().Context(), pullAuth, images)
	if err != nil {
		return err
	}
	defer tar.Close()

	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="images.tar"`)
	return c.Stream(http.StatusOK, "application/x-tar", tar)
}

func (s *Server) pullAndSaveImages(ctx context.Context, authn auth.Authenticator, images []string) (io.ReadCloser, error) {
	for _, img := range images {
		encodedAuth, err := auth.RegistryAuthFor(authn, img)
		if err != nil {
			return nil, err
		}
		rc, err := s.DockerClient.ImagePull(ctx, img, types.ImagePullOptions{
			RegistryAuth: encodedAuth,
		})
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		io.Copy(os.Stdout, rc)
	}

	return s.DockerClient.ImageSave(ctx, images)
}

func authFromFormFile(c echo.Context, filename string) (auth.Authenticator, error) {
	authFile, err := c.FormFile(filename)
	if err != nil {
		c.Logger().Info("no authentication info provided")
		return auth.EmptyAuthenticator, nil
	}
	authSrc, err := authFile.Open()
	if err != nil {
		return nil, err
	}
	defer authSrc.Close()

	return auth.FromReader(authSrc)
}

func env2Lvl(key string) log.Lvl {
	switch strings.ToLower(os.Getenv(key)) {
	case "debug":
		return log.DEBUG
	case "warn":
		return log.WARN
	case "error":
		return log.ERROR
	case "off":
		return log.OFF
	default:
		return log.INFO
	}
}

func envOrDefault(key, dflt string) string {
	if e := os.Getenv(key); e != "" {
		return e
	}
	return dflt
}
