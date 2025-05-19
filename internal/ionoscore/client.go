package ionoscore

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	sdk "github.com/ionos-developer/dns-sdk-go"
	log "github.com/sirupsen/logrus"

	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionos"
)

// DnsClient client of the dns api
type DnsClient struct {
	client *sdk.APIClient
}

// DnsService interface to the dns backend, also needed for creating mocks in tests
type DnsService interface {
	GetZones(ctx context.Context) ([]sdk.Zone, error)
	GetZone(ctx context.Context, zoneId string) (*sdk.CustomerZone, error)
	CreateRecords(ctx context.Context, zoneId string, records []sdk.Record) error
	DeleteRecord(ctx context.Context, zoneId string, recordId string) error
}

func IONOSCoreClient(config *ionos.Configuration) DnsService {
	maskAPIKey := func() string {
		if len(config.APIKey) <= 3 {
			return strings.Repeat("*", len(config.APIKey))
		}
		return fmt.Sprintf("%s%s", config.APIKey[:3], strings.Repeat("*", len(config.APIKey)-3))
	}
	log.Infof(
		"Creating ionos core DNS client with parameters: API Endpoint URL: '%v', Auth header: '%v', API key: '%v', Debug: '%v'",
		config.APIEndpointURL,
		config.AuthHeader,
		maskAPIKey(),
		config.Debug,
	)
	if config.DryRun {
		log.Warnf("*** Dry run is enabled, no changes will be made to ionos core DNS ***")
	}

	sdkConfig := sdk.NewConfiguration()
	if config.APIEndpointURL != "" {
		sdkConfig.Servers[0].URL = config.APIEndpointURL
	}
	sdkConfig.AddDefaultHeader(config.AuthHeader, config.APIKey)
	sdkConfig.UserAgent = fmt.Sprintf(
		"external-dns os %s arch %s",
		runtime.GOOS, runtime.GOARCH)
	sdkConfig.Debug = config.Debug

	return DnsClient{sdk.NewAPIClient(sdkConfig)}
}

// GetZones client get zones method
func (c DnsClient) GetZones(ctx context.Context) ([]sdk.Zone, error) {
	zones, _, err := c.client.ZonesApi.GetZones(ctx).Execute()
	return zones, err
}

// GetZone client get zone method
func (c DnsClient) GetZone(ctx context.Context, zoneId string) (*sdk.CustomerZone, error) {
	zoneInfo, _, err := c.client.ZonesApi.GetZone(ctx, zoneId).Execute()
	return zoneInfo, err
}

// CreateRecords client create records method
func (c DnsClient) CreateRecords(ctx context.Context, zoneId string, records []sdk.Record) error {
	_, _, err := c.client.RecordsApi.CreateRecords(ctx, zoneId).Record(records).Execute()
	return err
}

// DeleteRecord client delete record method
func (c DnsClient) DeleteRecord(ctx context.Context, zoneId string, recordId string) error {
	_, err := c.client.RecordsApi.DeleteRecord(ctx, zoneId, recordId).Execute()
	return err
}
