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
	"github.com/mikefarah/yq/v3/pkg/yqlib"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

var (
	repoFile    = os.Getenv("HELM_REPO_CONFIG_FILE")
	repoCache   = os.Getenv("HELM_REPO_CACHE_DIR")
	downloadDir = os.Getenv("HELM_DOWNLOAD_DIR")
	getters     = getter.All(&cli.EnvSettings{
		RepositoryConfig: repoFile,
		RepositoryCache:  repoCache,
	})
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
	//err := os.MkdirAll(filepath.Dir(repoFile), os.ModePerm)
	//if err != nil && !os.IsExist(err) {
	//	return err
	//}

	e := echo.New()
	s := &Server{e, opt.DockerClient}

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
	g.POST("/helm/repo", s.postHelmRepo)
	g.POST("/helm/chart", s.postHelmChart)

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
		return fmt.Errorf("Please specify a repository name using the name form param")
	}
	if url == "" {
		return fmt.Errorf("Please specify a repository URL using the url form param")
	}

	// The following code is mostly plugged from https://github.com/helm/helm/blob/master/cmd/helm/repo_add.go#L85
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
	_, err := c.FormFile("values.yaml")
	if err != nil {
		return err
	}

	dl := downloader.ChartDownloader{
		Out:              os.Stderr,
		RepositoryConfig: repoFile,
		RepositoryCache:  repoCache,
		Getters:          getters,
	}
	chrtRef := c.FormValue("chart")
	chrtVer := c.FormValue("version")
	destFile, _, err := dl.DownloadTo(chrtRef, chrtVer, downloadDir)
	if err != nil {
		return err
	}

	c.Logger().Info(destFile)
	return nil

	//var images []string
	//return s.streamImages(c, auth.EmptyAuthenticator, normalizeImages(images))
}

func findImagesInHelmChart(chrt *chart.Chart, values map[string]interface{}) ([]string, error) {
	return nil, nil
	//rendered, err := engine.New().Render(chrt, values)
	//if err != nil {
	//	return nil, err
	//}
	//var images = make([]string, len(rendered))
	//for filename, contents := range rendered {
	//	if !strings.HasSuffix(filename, "yaml") && !strings.HasSuffix(filename, "yml") {
	//		continue
	//	}
	//	nodes, err := findImageNodeInYaml(contents)
	//	if err != nil {
	//		return nil, err
	//	}
	//	images = append(images, nodes...)
	//}
	//return images, nil
}

func findImageNodeInYaml(yml string) ([]string, error) {
	var node yaml.Node
	err := yaml.Unmarshal([]byte(yml), &node)
	nodes, err := yqlib.NewYqLib().Get(&node, "**.image", false)
	if err != nil {
		return nil, err
	}
	var result []string
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
