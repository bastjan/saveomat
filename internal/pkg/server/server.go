package server

import (
	"bufio"
	"context"
	"embed"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/bastjan/saveomat/internal/pkg/auth"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/sync/errgroup"
)

//go:embed public/*
var publicContent embed.FS

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

	// Redirect /base -> /base/
	if baseurl != "" {
		e.GET(baseurl, func(c echo.Context) error {
			return c.Redirect(http.StatusPermanentRedirect, baseurl+"/")
		})
	}

	g := e.Group(baseurl)
	g.POST("/tar", func(c echo.Context) error {
		err := s.postTar(c)
		if err != nil {
			return dockerToEchoErrorMapping(err)
		}
		return nil
	})
	g.GET("/tar", func(c echo.Context) error {
		err := s.getTar(c)
		if err != nil {
			return dockerToEchoErrorMapping(err)
		}
		return nil
	})
	// Static files for web gui
	g.GET("/*", echo.WrapHandler(http.FileServer(http.FS(publicContent))), middleware.Rewrite(map[string]string{baseurl + "/*": "/public/$1"}))

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

	g, errCtx := errgroup.WithContext(ctx)

	for _, img := range images {
		img := img
		g.Go(func() error {
			encodedAuth, err := auth.RegistryAuthFor(authn, img)
			if err != nil {
				return err
			}
			rc, err := s.DockerClient.ImagePull(errCtx, img, types.ImagePullOptions{
				RegistryAuth: encodedAuth,
			})
			if err != nil {
				return err
			}
			defer rc.Close()
			io.Copy(os.Stdout, rc)
			return nil
		})

	}

	if err := g.Wait(); err != nil {
		return nil, err
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

func dockerToEchoErrorMapping(err error) *echo.HTTPError {
	var status int
	switch {
	case strings.Contains(err.Error(), "forbidden"):
		status = http.StatusForbidden
	case strings.Contains(err.Error(), "not found"):
		status = http.StatusNotFound
	case strings.Contains(err.Error(), "unauthorised"):
		status = http.StatusUnauthorized
	case strings.Contains(err.Error(), "service unavailable"):
		status = http.StatusServiceUnavailable
	case strings.Contains(err.Error(), "bad request"):
		status = http.StatusBadRequest
	case strings.Contains(err.Error(), "bad gateway"):
		status = http.StatusBadGateway
	case strings.Contains(err.Error(), "request timeout"):
		status = http.StatusRequestTimeout
	default:
		status = http.StatusInternalServerError
	}
	return echo.NewHTTPError(status, err.Error())
}
