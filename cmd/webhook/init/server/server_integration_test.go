//go:build integration

package server_test

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/configuration"
	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/dnsprovider"
	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/logging"
	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/server"
	"github.com/ionos-cloud/external-dns-ionos-webhook/pkg/webhook"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"testing"
)

const kindConfigFile = "../../../../build/kind/config"

func startDnsMockserver(cli *client.Client) (container.CreateResponse, error) {
	hostConfig := &container.HostConfig{
		PortBindings: map[nat.Port][]nat.PortBinding{
			nat.Port("1080/tcp"): {
				{
					HostIP:   "0.0.0.0",
					HostPort: "1080",
				},
			},
		},
	}
	msCreate, err := cli.ContainerCreate(context.Background(), &container.Config{
		Image: "mockserver/mockserver:5.15.0",
		Env:   []string{"MOCKSERVER_LOG_LEVEL=DEBUG"},
	}, hostConfig, nil, nil, "mockserver")
	if err != nil {
		return container.CreateResponse{}, err
	}
	if err := cli.ContainerStart(context.Background(), msCreate.ID, container.StartOptions{}); err != nil {
		return container.CreateResponse{}, err
	}
	return msCreate, nil
}

func startExternalDNS(cli *client.Client) (container.CreateResponse, error) {
	absKindConfigFile, err := filepath.Abs(kindConfigFile)
	if err != nil {
		return container.CreateResponse{}, fmt.Errorf("error getting absolute path: %v", err)
	}
	hostConfig := &container.HostConfig{
		NetworkMode: "host",
		Binds:       []string{absKindConfigFile + ":/root/.kube/config"},
	}
	msCreate, err := cli.ContainerCreate(context.Background(), &container.Config{
		Image: "registry.k8s.io/external-dns/external-dns:v0.15.0",
		Cmd: []string{
			"--source=ingress", "--provider=webhook", "--log-level=debug",
		},
	}, hostConfig, nil, nil, "external-dns")
	if err != nil {
		return container.CreateResponse{}, err
	}
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
	// check if kindConfigFile exists
	if _, err := os.Stat(kindConfigFile); os.IsNotExist(err) {
		log.Fatalf("Kind config file not found, is kind running? : %v", err)
	}
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Error creating docker client: %v", err)
	}
	dnsMockserverCreate, err := startDnsMockserver(dockerClient)
	if err != nil {
		log.Fatalf("Error starting mock server: %v", err)
	}
	srv, err := startWebhookServer()
	if err != nil {
		err = cleanUp(dockerClient, nil, []string{dnsMockserverCreate.ID})
		if err != nil {
			log.Errorf("Error cleaning up: %v", err)
		}
		log.Fatalf("Error starting webhook server: %v", err)
	}
	// wait a little bit until the servers are up
	time.Sleep(3 * time.Second)
	externalDNSCreate, err := startExternalDNS(dockerClient)
	if err != nil {
		err = cleanUp(dockerClient, srv, []string{dnsMockserverCreate.ID})
		if err != nil {
			log.Errorf("Error cleaning up: %v", err)
		}
		log.Fatalf("Error starting external-dns: %v", err)
	}

	m.Run()

	err = cleanUp(dockerClient, srv, []string{dnsMockserverCreate.ID, externalDNSCreate.ID})
	if err != nil {
		log.Errorf("Error cleaning up: %v", err)
	}
}

func cleanUp(dockerClient *client.Client, webhookServer *http.Server, containerIds []string) error {
	if webhookServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := webhookServer.Shutdown(ctx); err != nil {
			cancel()
			return err
		}
		cancel()
	}
	for _, id := range containerIds {
		err := removeContainer(dockerClient, id)
		if err != nil {
			return err
		}
	}
	return nil
}

func startWebhookServer() (*http.Server, error) {
	// set environment variables
	err := os.Setenv("LOG_LEVEL", "debug")
	err = os.Setenv("LOG_FORMAT", "text")
	err = os.Setenv("SERVER_HOST", "localhost")
	err = os.Setenv("SERVER_PORT", "8888")
	err = os.Setenv("SERVER_READ_TIMEOUT", "")
	err = os.Setenv("SERVER_WRITE_TIMEOUT", "")
	err = os.Setenv("DOMAIN_FILTER", "")
	err = os.Setenv("EXCLUDE_DOMAIN_FILTER", "")
	err = os.Setenv("REGEXP_DOMAIN_FILTER", "")
	err = os.Setenv("REGEXP_DOMAIN_FILTER_EXCLUSION", "")
	err = os.Setenv("IONOS_API_KEY", "some-api-key")
	err = os.Setenv("IONOS_API_URL", "http://localhost:1080") // mock server
	err = os.Setenv("IONOS_AUTH_HEADER", "")
	err = os.Setenv("IONOS_DEBUG", "true")
	err = os.Setenv("DRY_RUN", "false")
	if err != nil {
		return nil, err
	}
	// Start the webhook server
	logging.Init()
	config := configuration.Init()
	provider, err := dnsprovider.Init(config)
	if err != nil {
		return nil, err
	}
	srv := server.Init(config, webhook.New(provider))
	return srv, nil
}

func TestServerIntegration(t *testing.T) {
	fmt.Printf("TestServerIntegration\n")
	time.Sleep(10 * time.Second)
}
