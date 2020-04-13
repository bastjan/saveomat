package main

import (
	"github.com/bastjan/saveomat/internal/pkg/server"
	"github.com/docker/docker/client"
)

func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic("Could not initialize docker client.")
	}

	e := server.NewServer(cli)
	e.Logger.Fatal(e.Start(":8080"))
}
