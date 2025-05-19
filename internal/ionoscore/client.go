package ionoscore

import (
	"context"

	sdk "github.com/ionos-developer/dns-sdk-go"
)

// DnsService interface to the dns backend, also needed for creating mocks in tests
type DnsService interface {
	GetZones(ctx context.Context) ([]sdk.Zone, error)
	GetZone(ctx context.Context, zoneId string) (*sdk.CustomerZone, error)
	CreateRecords(ctx context.Context, zoneId string, records []sdk.Record) error
	DeleteRecord(ctx context.Context, zoneId string, recordId string) error
}

// DnsClient client of the dns api
type DnsClient struct {
	client *sdk.APIClient
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
