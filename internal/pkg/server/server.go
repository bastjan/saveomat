package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/bastjan/saveomat/internal/pkg/auth"
	"github.com/bastjan/saveomat/internal/pkg/helmchart"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/repo"
)

type ServerOpts struct {
	BaseURL      string
	DockerClient client.ImageAPIClient
}

type Server struct {
	*echo.Echo
	DockerClient client.ImageAPIClient
	HelmClient   helmchart.Renderer
}

func NewServer(opt ServerOpts) *Server {
	e := echo.New()
	s := &Server{e, opt.DockerClient, helmchart.NewRenderer()}
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
	g.POST("/helm/tar", s.postHelm)

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

func (s *Server) postHelm(c echo.Context) error {
	// Chart settings
	repoName := c.FormValue("repoName")
	if repoName == "" {
		return fmt.Errorf("repository name not set")
	}
	repoURL := c.FormValue("repoURL")
	if repoURL == "" {
		return fmt.Errorf("repository URL not set")
	}
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
	_, verify := c.QueryParams()["verify"]
	target := helmchart.RenderTarget{
		RepoConfig: repo.Entry{
			Name:                  repoName,
			URL:                   repoURL,
			Username:              c.FormValue("username"),
			Password:              c.FormValue("password"),
			InsecureSkipTLSverify: false,
		},
		Verify:         verify,
		ChartReference: chrtRef,
		ChartVersion:   chrtVer,
		Values:         values,
	}

	authn, err := authFromFormFile(c, "config.json")
	if err != nil {
		return err
	}

	manifest, err := s.HelmClient.Render(target)
	if err != nil {
		return err
	}
	images, err := helmchart.FindImagesInManifest(manifest)
	if err != nil {
		return err
	}
	normalized := normalizeImages(images)
	c.Logger().Info(fmt.Sprintf("Found %d images: %s", len(normalized), normalized))
	return s.streamImages(c, authn, normalized)
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
