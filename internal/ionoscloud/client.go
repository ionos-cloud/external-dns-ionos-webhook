package ionoscloud

import (
	"context"

	sdk "github.com/ionos-cloud/sdk-go-dns"
	log "github.com/sirupsen/logrus"
)

type DNSClient struct {
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

// GetZoneRecords retrieve all records for a zone https://github.com/ionos-cloud/sdk-go-dns/blob/master/docs/api/RecordsApi.md#recordsget
func (c *DNSClient) GetZoneRecords(ctx context.Context, offset int32, zoneId string) (sdk.RecordReadList, error) {
	log.Debugf("get all records for zone '%s' with offset %d ...", zoneId, offset)
	records, _, err := c.client.RecordsApi.RecordsGet(ctx).FilterZoneId(zoneId).Limit(recordReadLimit).Offset(offset).
		FilterState(sdk.PROVISIONINGSTATE_AVAILABLE).Execute()
	if err != nil {
		log.Errorf("failed to get records for zone '%s': %v", zoneId, err)
		return records, err
	}
	if records.HasItems() {
		log.Debugf("found %d records for zone '%s'", len(*records.Items), zoneId)
	} else {
		log.Debugf("no records found for zone '%s'", zoneId)
	}
	return records, err
}

func (c *DNSClient) GetRecordsByZoneIdAndName(ctx context.Context, zoneId, name string) (sdk.RecordReadList, error) {
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
func (c *DNSClient) GetZones(ctx context.Context, offset int32) (sdk.ZoneReadList, error) {
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
func (c *DNSClient) CreateRecord(ctx context.Context, zoneId string, record sdk.RecordCreate) error {
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
func (c *DNSClient) DeleteRecord(ctx context.Context, zoneId string, recordId string) error {
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
