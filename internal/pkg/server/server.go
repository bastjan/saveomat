package server

import (
	"bufio"
	"context"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Server struct {
	*echo.Echo
	DockerClient client.ImageAPIClient
}

func NewServer(c client.ImageAPIClient) *Server {
	e := echo.New()
	s := &Server{e, c}

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("512K"))

	e.Static("/", "public")
	e.POST("/tar", s.postTar)
	e.GET("/tar", s.getTar)

	return s
}

func (s *Server) getTar(c echo.Context) error {
	p := c.QueryParams()
	images, ok := p["image"]
	if !ok {
		return c.NoContent(http.StatusBadRequest)
	}

	return s.streamImages(c, normalizeImages(images))
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

	images := make([]string, 0, 5)
	sc := bufio.NewScanner(src)
	for sc.Scan() {
		images = append(images, sc.Text())
	}
	if sc.Err() != nil {
		return sc.Err()
	}

	return s.streamImages(c, normalizeImages(images))
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

func (s *Server) streamImages(c echo.Context, images []string) error {
	if len(images) == 0 {
		return c.NoContent(http.StatusBadRequest)
	}

	tar, err := s.pullAndSaveImages(c.Request().Context(), images)
	if err != nil {
		return err
	}
	defer tar.Close()

	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="images.tar"`)
	return c.Stream(http.StatusOK, mime.TypeByExtension(".tar"), tar)
}

func (s *Server) pullAndSaveImages(ctx context.Context, images []string) (io.ReadCloser, error) {
	for _, img := range images {
		rc, err := s.DockerClient.ImagePull(ctx, img, types.ImagePullOptions{})
		if err != nil {
			panic(err)
		}
		defer rc.Close()
		io.Copy(os.Stdout, rc)
	}

	return s.DockerClient.ImageSave(ctx, images)
}
