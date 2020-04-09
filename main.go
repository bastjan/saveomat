package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
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

	e.Static("/", "public")
	e.POST("/tar", postTar)

	e.Logger.Fatal(e.Start(":8080"))

	imgTar, err := getImages(context.TODO(), images)
	if err != nil {
		panic(err)
	}
	defer imgTar.Close()

	f, err := ioutil.TempFile(".", "images-*.tar")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	io.Copy(f, imgTar)
}

func postTar(c echo.Context) error {
	file, err := c.FormFile("images.txt")
	if err != nil {
		return err
	}
	if file.Size > 512*1024 {
		return fmt.Errorf("input too large: %d bytes, allowed are %d bytes", file.Size, 512*1024)
	}
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	images := make([]string, 0, 5)

	s := bufio.NewScanner(src)
	for s.Scan() {
		image := strings.Trim(s.Text(), " ")
		if image == "" || strings.HasPrefix(image, "#") {
			continue
		}
		images = append(images, s.Text())
	}
	if s.Err() != nil {
		return s.Err()
	}

	tar, err := getImages(c.Request().Context(), images)
	if err != nil {
		return err
	}
	defer tar.Close()

	return c.Stream(http.StatusOK, mime.TypeByExtension(".tar"), tar)
}

func getImages(ctx context.Context, images []string) (io.ReadCloser, error) {
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
