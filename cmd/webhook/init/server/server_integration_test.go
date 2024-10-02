//go:build integration

package server_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/configuration"
	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/dnsprovider"
	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/logging"
	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/server"
	"github.com/ionos-cloud/external-dns-ionos-webhook/pkg/webhook"
	mockserverClient "github.com/ionos-cloud/mockserver-client-go/pkg/client"
	sdk "github.com/ionos-cloud/sdk-go-dns"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const jwtToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxMjMsImVtYWlsIjoidXNlckBleGFtcGxlLmNvbSIsImV4cCI6MTYwOTcyMzQ2MCwiaWF0IjoxNjA5NzIyODYwfQ.nKZ8eIGFEnkCZ4yarPPde23hYzLHhqn9Od_L-X0jf0g"

const kindConfigFile = "../../../../build/kind/config"

var (
	k8sClient     *kubernetes.Clientset
	dnsMockClient *mockserverClient.MockserverClient
)

func createK8sClient(kindConfigFile string) (*kubernetes.Clientset, error) {
	// Get the absolute path for kindConfigFile
	absKindConfigFile, err := filepath.Abs(kindConfigFile)
	if err != nil {
		return nil, fmt.Errorf("error getting absolute path: %v", err)
	}

	// Load the kubeconfig file
	config, err := clientcmd.BuildConfigFromFlags("", absKindConfigFile)
	if err != nil {
		return nil, fmt.Errorf("error loading kubeconfig: %v", err)
	}

	// Create the Kubernetes client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating Kubernetes client: %v", err)
	}

	return clientset, nil
}

func startDnsMockserver(cli *client.Client) (*container.CreateResponse, error) {
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
		return &container.CreateResponse{}, err
	}
	if err := cli.ContainerStart(context.Background(), msCreate.ID, container.StartOptions{}); err != nil {
		return &container.CreateResponse{}, err
	}
	return &msCreate, nil
}

func startExternalDNS(cli *client.Client) (*container.CreateResponse, error) {
	absKindConfigFile, err := filepath.Abs(kindConfigFile)
	if err != nil {
		return &container.CreateResponse{}, fmt.Errorf("error getting absolute path: %v", err)
	}
	hostConfig := &container.HostConfig{
		NetworkMode: "host",
		Binds:       []string{absKindConfigFile + ":/root/.kube/config"},
	}
	msCreate, err := cli.ContainerCreate(context.Background(), &container.Config{
		Image: "registry.k8s.io/external-dns/external-dns:v0.15.0",
		Cmd: []string{
			"--source=ingress", "--source=service", "--provider=webhook", "--log-level=debug",
		},
	}, hostConfig, nil, nil, "external-dns")
	if err != nil {
		return &container.CreateResponse{}, err
	}
	if err := cli.ContainerStart(context.Background(), msCreate.ID, container.StartOptions{}); err != nil {
		return &container.CreateResponse{}, err
	}
	return &msCreate, nil
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
	var err error
	var dockerClient *client.Client
	var dnsMockServerCreate *container.CreateResponse
	var externalDNSCreate *container.CreateResponse
	var webhookServer *http.Server
	defer func() {
		var ids []string
		if dnsMockServerCreate != nil {
			ids = append(ids, dnsMockServerCreate.ID)
		}
		if externalDNSCreate != nil {
			ids = append(ids, externalDNSCreate.ID)
		}
		cleanUp(dockerClient, ids, webhookServer)
	}()

	_, err = os.Stat(kindConfigFile)
	if os.IsNotExist(err) {
		log.Errorf("Kind config file not found, is kind running? : %v", err)
		return
	}

	k8sClient, err = createK8sClient(kindConfigFile)
	if err != nil {
		log.Errorf("Error creating k8s client: %v", err)
		return
	}

	dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Errorf("Error creating docker client: %v", err)
		return
	}

	dnsMockServerCreate, err = startDnsMockserver(dockerClient)
	if err != nil {
		log.Errorf("Error starting mock server: %v", err)
		return
	}
	time.Sleep(1 * time.Second)
	dnsMockClient, err = mockserverClient.NewMockServerClient()
	if err != nil {
		log.Errorf("Error creating mock server client: %v", err)
		return
	}

	err = initialDNSProviderSetup()
	if err != nil {
		log.Errorf("Error setting up stubs: %v", err)
		return
	}
	webhookServer, err = startWebhookServer()
	if err != nil {
		log.Errorf("Error starting webhook server: %v", err)
		return
	}
	// wait a little bit until the servers are up
	time.Sleep(3 * time.Second)
	externalDNSCreate, err = startExternalDNS(dockerClient)
	if err != nil {
		log.Errorf("Error starting external-dns: %v", err)
		return
	}

	m.Run()
}

func cleanUp(dockerClient *client.Client, containerIds []string, webhookServer *http.Server) {
	if dockerClient != nil {
		for _, id := range containerIds {
			log.Debugf("Cleaning up: removing container with id '%s'", id)
			err := removeContainer(dockerClient, id)
			if err != nil {
				log.Errorf("Error in cleaning up, can't remove container with id '%s' :  %v", id, err)
			}
		}
	}
	if webhookServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		log.Debugf("Cleaning up: shutting down webhook server")
		if err := webhookServer.Shutdown(ctx); err != nil {
			cancel()
			log.Errorf("Error in cleaning up, can't shut down the webhook server:  %v", err)
		}
		cancel()
	}
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
	err = os.Setenv("IONOS_API_KEY", jwtToken)
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

func initialDNSProviderSetup() error {
	err := setupDNSProviderRecords(sdk.NewRecordReadList("someid", "collection", "href", []sdk.RecordRead{}, 0, 100, sdk.Links{}))
	if err != nil {
		return err
	}
	// accept POST requests to create records
	recordRead := sdk.NewRecordRead("someid", "record", "href", sdk.MetadataWithStateFqdnZoneId{}, sdk.Record{
		Name:    ptr("some-name"),
		Type:    ptr("A"),
		Content: ptr("some-content"),
	})
	dnsMockClient.When(
		mockserverClient.Request().
			WithMethod(http.MethodPost).
			WithPath("/zones/{zoneId}/records").
			WithPathParameter("zoneId", mockserverClient.PatternValue("^.+$")).
			WithExactHeader("Accept", "application/json"),
	).ThenMustRespond(
		mockserverClient.Response().
			WithStatusCode(http.StatusAccepted).
			WithHeader("Content-Type", "application/json").
			WithBody(recordRead),
	)
	return nil
}

func setupDNSProviderRecords(currentRecords *sdk.RecordReadList) error {
	dnsMockClient.When(
		mockserverClient.Request().
			WithMethod(http.MethodGet).
			WithPath("/records").
			WithExactHeader("Accept", "application/json"),
	).ThenMustRespond(
		mockserverClient.Response().
			WithStatusCode(http.StatusOK).
			WithHeader("Content-Type", "application/json").
			WithBody(currentRecords),
	)
	return nil
}

func setupDNSProviderWithZonesNames(zoneNames []string) ([]string, error) {
	return setupDNSProviderZones(lo.Map(zoneNames, func(zoneName string, _ int) sdk.Zone {
		return sdk.Zone{
			ZoneName: ptr(zoneName),
		}
	}))
}

func setupDNSProviderZones(zones []sdk.Zone) ([]string, error) {
	zoneList := sdk.NewZoneReadList("someid", "collection", "href", 0, 100, sdk.Links{},
		lo.Map(zones, func(zone sdk.Zone, _ int) sdk.ZoneRead {
			return sdk.ZoneRead{
				Id:   ptr(uuid.NewString()),
				Type: ptr("collection"),
				Href: ptr("href"),
				Metadata: ptr(sdk.MetadataWithStateNameservers{
					State:       ptr(sdk.PROVISIONINGSTATE_AVAILABLE),
					Nameservers: ptr([]string{"nameserver1", "nameserver1"}),
				}),
				Properties: ptr(zone),
			}
		}))
	dnsMockClient.When(
		mockserverClient.Request().
			WithMethod(http.MethodGet).
			WithPath("/zones").
			WithExactHeader("Accept", "application/json"),
	).ThenMustRespond(
		mockserverClient.Response().
			WithStatusCode(http.StatusOK).
			WithHeader("Content-Type", "application/json").
			WithBody(zoneList),
	)
	items := *zoneList.Items
	ids := lo.Map(items, func(zone sdk.ZoneRead, _ int) string {
		return *zone.Id
	})
	return ids, nil
}

func createK8sServiceWithHostAnnotation(hostName string) (*v1.Service, error) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-service",
			Annotations: map[string]string{
				"external-dns.alpha.kubernetes.io/internal-hostname": hostName,
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(80),
				},
			},
			Type: v1.ServiceTypeClusterIP,
		},
	}

	svc, err := k8sClient.CoreV1().Services("default").Create(context.Background(), service, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("error creating service: %v", err)
	}
	return svc, nil
}

func deleteK8sService(service *v1.Service) error {
	return k8sClient.CoreV1().Services("default").Delete(context.Background(), service.Name, metav1.DeleteOptions{})
}

func TestAddService(t *testing.T) {
	recordDomain := "test.org"
	recordSubdomain := "test-service"
	// given: zone must exist
	zoneIds, err := setupDNSProviderWithZonesNames([]string{recordDomain})
	require.NoError(t, err)

	// when k8s service is created with annotation
	svc, err := createK8sServiceWithHostAnnotation(recordSubdomain + "." + recordDomain)
	defer func() {
		err := deleteK8sService(svc)
		require.NoError(t, err)
	}()
	require.NoError(t, err)
	// then a post record request is sent
	thenRequestBody := fmt.Sprintf(`
{ 
	"properties": {
		"content":"%s",
		"enabled":true,
		"name":"%s",
		"ttl":3600,
		"type":"A"
	}
}`, svc.Spec.ClusterIP, recordSubdomain)

	// after 60s the request should be sent
	time.Sleep(3 * time.Second)
	require.True(t, dnsMockClient.MustVerify(
		mockserverClient.Request().WithMethod(http.MethodPost).
			WithPath("/zones/"+zoneIds[0]+"/records").
			WithBody(
				mockserverClient.JsonBodyMatch(thenRequestBody, mockserverClient.MatchTypeStrict)),
		mockserverClient.CalledAtLeast(1)))
}

func ptr[T any](v T) *T {
	return &v
}
