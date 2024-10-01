//go:build integration

package server_test

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"time"

	"testing"
)

func startMockServer(cli *client.Client) (container.CreateResponse, error) {
	// Define port bindings
	hostConfig := &container.HostConfig{
		PortBindings: map[nat.Port][]nat.PortBinding{
			"1080/tcp": {
				{
					HostIP:   "0.0.0.0",
					HostPort: "1080",
				},
			},
		},
	}

	// Create the container with port bindings
	msCreate, err := cli.ContainerCreate(context.Background(), &container.Config{
		Image: "mockserver/mockserver:5.15.0",
		Env:   []string{"MOCKSERVER_LOG_LEVEL=DEBUG"},
	}, hostConfig, nil, nil, "mockserver")
	if err != nil {
		return container.CreateResponse{}, err
	}

	//Start the container
	if err := cli.ContainerStart(context.Background(), msCreate.ID, container.StartOptions{}); err != nil {
		return container.CreateResponse{}, err
	}
	return msCreate, nil
}

func removeContainer(cli *client.Client, id string) error {
	if err := cli.ContainerStop(context.Background(), id, container.StopOptions{}); err != nil {
		return err
	}
	if err := cli.ContainerRemove(context.Background(), id, container.RemoveOptions{}); err != nil {
		return err
	}
	return nil
}

func TestMain(m *testing.M) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	msCreate, err := startMockServer(dockerClient)
	if err != nil {
		panic(err)
	}
	m.Run()
	err = removeContainer(dockerClient, msCreate.ID)
	if err != nil {
		panic(err)
	}
}

func startWebhookServer() error {
	// Start the webhook server
	return nil
}

func TestServerIntegration(t *testing.T) {
	fmt.Printf("TestServerIntegration\n")
	time.Sleep(10 * time.Second)
}
