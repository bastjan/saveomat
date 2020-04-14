package main

import (
	"os"

	"github.com/bastjan/saveomat/internal/pkg/server"
	"github.com/docker/docker/client"
)

func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic("Could not initialize docker client.")
	}

	e := server.NewServer(server.ServerOpts{
		DockerClient: cli,
		BaseURL:      os.Getenv("BASE_URL"),
	})

	e.Logger.Fatal(e.Start(":8080"))
}
