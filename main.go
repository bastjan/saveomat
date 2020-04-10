package main

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

var images = []string{"busybox", "alpine"}

func main() {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("512K"))

	e.Static("/", "public")
	e.POST("/tar", postTar)
	e.GET("/tar", getTar)

	e.Logger.Fatal(e.Start(":8080"))
}

func getTar(c echo.Context) error {
	p := c.QueryParams()
	images, ok := p["image"]
	if !ok {
		return c.NoContent(http.StatusBadRequest)
	}

	return streamImages(c, normalizeImages(images))
}

func postTar(c echo.Context) error {
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
	s := bufio.NewScanner(src)
	for s.Scan() {
		images = append(images, s.Text())
	}
	if s.Err() != nil {
		return s.Err()
	}

	return streamImages(c, normalizeImages(images))
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

func streamImages(c echo.Context, images []string) error {
	if len(images) == 0 {
		return c.NoContent(http.StatusBadRequest)
	}

	tar, err := pullAndSaveImages(c.Request().Context(), images)
	if err != nil {
		return err
	}
	defer tar.Close()

	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="images.tar"`)
	return c.Stream(http.StatusOK, mime.TypeByExtension(".tar"), tar)
}

func pullAndSaveImages(ctx context.Context, images []string) (io.ReadCloser, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	for _, img := range images {
		rc, err := cli.ImagePull(ctx, img, types.ImagePullOptions{})
		if err != nil {
			panic(err)
		}
		defer rc.Close()
		io.Copy(os.Stdout, rc)
	}

	return cli.ImageSave(ctx, images)
}
