package server

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/bastjan/saveomat/internal/pkg/auth"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
)

const (
	repoConfig = "/tmp/helm/repositories.yaml"
	repoCache  = "/tmp/helm/foo"
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
	g.POST("/helm", s.postHelm)

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
	_, err := c.FormFile("values.yaml")
	if err != nil {
		return err
	}

	_ = downloader.ChartDownloader{
		Out:              os.Stderr,
		RepositoryConfig: repoConfig,
		RepositoryCache:  repoCache,
		Getters: getter.All(&cli.EnvSettings{
			RepositoryConfig: repoConfig,
			RepositoryCache:  repoCache,
		}),
	}

	var images []string
	return s.streamImages(c, auth.EmptyAuthenticator, normalizeImages(images))
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
