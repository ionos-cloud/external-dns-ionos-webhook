package ionoscloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	sdk "github.com/ionos-cloud/sdk-go-dns"
	log "github.com/sirupsen/logrus"

	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionos"
)

type DnsClient struct {
	client *sdk.APIClient
	dryRun bool
}

type DNSService interface {
	GetZoneRecords(ctx context.Context, offset int32, zoneId string) (sdk.RecordReadList, error)
	GetRecordsByZoneIdAndName(ctx context.Context, zoneId, name string) (sdk.RecordReadList, error)
	GetZones(ctx context.Context, offset int32) (sdk.ZoneReadList, error)
	DeleteRecord(ctx context.Context, zoneId string, recordId string) error
	CreateRecord(ctx context.Context, zoneId string, record sdk.RecordCreate) error
}

func CreateClient(ionosConfig *ionos.Configuration) DNSService {
	jwtString := func() string {
		split := strings.Split(ionosConfig.APIKey, ".")
		if len(split) == 3 {
			headerBytes, _ := base64.RawStdEncoding.DecodeString(split[0])
			payloadBytes, _ := base64.RawStdEncoding.DecodeString(split[1])
			return fmt.Sprintf("JWT-header: %s, JWT-payload: %s", headerBytes, payloadBytes)
		}
		return ""
	}
	log.Infof(
		"Creating ionos cloud DNS client with parameters: API Endpoint URL: '%v', Auth header: '%v', Debug: '%v'",
		ionosConfig.APIEndpointURL,
		ionosConfig.AuthHeader,
		ionosConfig.Debug,
	)
	log.Debugf("JWT: %s", jwtString())

	if ionosConfig.DryRun {
		log.Warnf("*** Dry run is enabled, no changes will be made to ionos cloud DNS ***")
	}

	sdkConfig := sdk.NewConfiguration("", "", ionosConfig.APIKey, ionosConfig.APIEndpointURL)
	sdkConfig.Debug = ionosConfig.Debug
	return &DnsClient{sdk.NewAPIClient(sdkConfig), ionosConfig.DryRun}
}

// GetZoneRecords retrieve all records for a zone https://github.com/ionos-cloud/sdk-go-dns/blob/master/docs/api/RecordsApi.md#recordsget
func (c *DnsClient) GetZoneRecords(ctx context.Context, offset int32, zoneId string) (sdk.RecordReadList, error) {
	log.Debugf("get all records for zone with id '%s' with offset %d ...", zoneId, offset)
	records, _, err := c.client.RecordsApi.RecordsGet(ctx).FilterZoneId(zoneId).Limit(recordReadLimit).Offset(offset).
		FilterState(sdk.PROVISIONINGSTATE_AVAILABLE).Execute()
	if err != nil {
		log.Errorf("failed to get records for zone with id '%s': %v", zoneId, err)
		return records, err
	}
	if records.HasItems() {
		log.Debugf("found %d records for zone with id '%s'", len(*records.Items), zoneId)
	} else {
		log.Debugf("no records found for zone with id '%s'", zoneId)
	}
	return records, err
}

func (c *DnsClient) GetRecordsByZoneIdAndName(ctx context.Context, zoneId, name string) (sdk.RecordReadList, error) {
	logger := log.WithField(logFieldZoneID, zoneId).WithField(logFieldRecordName, name)
	logger.Debug("get records from zone by name ...")
	records, _, err := c.client.RecordsApi.RecordsGet(ctx).FilterZoneId(zoneId).FilterName(name).
		FilterState(sdk.PROVISIONINGSTATE_AVAILABLE).Execute()
	if err != nil {
		logger.Errorf("failed to get records from zone by name: %v", err)
		return records, err
	}
	if records.HasItems() {
		logger.Debugf("found %d records", len(*records.Items))
	} else {
		logger.Debug("no records found")
	}
	return records, nil
}

// GetZones client get zones method
func (c *DnsClient) GetZones(ctx context.Context, offset int32) (sdk.ZoneReadList, error) {
	log.Debug("get all zones ...")
	zones, _, err := c.client.ZonesApi.ZonesGet(ctx).Offset(offset).Limit(zoneReadLimit).FilterState(sdk.PROVISIONINGSTATE_AVAILABLE).Execute()
	if err != nil {
		log.Errorf("failed to get all zones: %v", err)
		return zones, err
	}
	if zones.HasItems() {
		log.Debugf("found %d zones", len(*zones.Items))
	} else {
		log.Debug("no zones found")
	}
	return zones, err
}

// CreateRecord client create record method
func (c *DnsClient) CreateRecord(ctx context.Context, zoneId string, record sdk.RecordCreate) error {
	recordProps := record.GetProperties()
	logger := log.WithField(logFieldZoneID, zoneId).WithField(logFieldRecordName, *recordProps.GetName()).
		WithField(logFieldRecordType, *recordProps.GetType()).WithField(logFieldRecordContent, *recordProps.GetContent()).
		WithField(logFieldRecordTTL, *recordProps.GetTtl())
	logger.Debugf("creating record ...")
	if !c.dryRun {
		recordRead, _, err := c.client.RecordsApi.ZonesRecordsPost(ctx, zoneId).RecordCreate(record).Execute()
		if err != nil {
			logger.Errorf("failed to create record: %v", err)
			return err
		}
		logger.Debugf("created successfully record with id: '%s'", *recordRead.GetId())
	} else {
		logger.Info("** DRY RUN **, record not created")
	}
	return nil
}

// DeleteRecord client delete record method
func (c *DnsClient) DeleteRecord(ctx context.Context, zoneId string, recordId string) error {
	logger := log.WithField(logFieldZoneID, zoneId).WithField(logFieldRecordID, recordId)
	logger.Debugf("deleting record: %v ...", recordId)
	if !c.dryRun {
		_, _, err := c.client.RecordsApi.ZonesRecordsDelete(ctx, zoneId, recordId).Execute()
		if err != nil {
			logger.Errorf("failed to delete record: %v", err)
			return err
		}
		logger.Debug("record deleted successfully")
	} else {
		logger.Info("** DRY RUN **, record not deleted")
	}
	return nil
}
