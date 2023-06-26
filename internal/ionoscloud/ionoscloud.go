package ionoscloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionos"
	"github.com/ionos-cloud/external-dns-ionos-webhook/pkg/endpoint"
	"github.com/ionos-cloud/external-dns-ionos-webhook/pkg/plan"
	"github.com/ionos-cloud/external-dns-ionos-webhook/pkg/provider"
	sdk "github.com/ionos-cloud/sdk-go-dns"
	log "github.com/sirupsen/logrus"
)

const (
	logFieldZoneID     = "zoneID"
	logFieldRecordID   = "recordID"
	logFieldRecordName = "recordName"
	// max number of records to read per request
	recordReadLimit = 1000
	// max number of records to read in total
	recordReadMaxCount = 10 * recordReadLimit
	// max number of zones to read per request
	zoneReadLimit = 1000
	// max number of zones to read in total
	zoneReadMaxCount = 10 * zoneReadLimit
)

type DNSClient struct {
	client *sdk.APIClient
	dryRun bool
}

type DNSService interface {
	GetAllRecords(ctx context.Context, offset int32) (sdk.RecordReadList, error)
	GetZoneRecords(ctx context.Context, zoneId string) (sdk.RecordReadList, error)
	GetRecordsByZoneIdAndName(ctx context.Context, zoneId, name string) (sdk.RecordReadList, error)
	GetZones(ctx context.Context, offset int32) (sdk.ZoneReadList, error)
	GetZone(ctx context.Context, zoneId string) (sdk.ZoneRead, error)
	DeleteRecord(ctx context.Context, zoneId string, recordId string) error
	CreateRecord(ctx context.Context, zoneId string, record sdk.RecordCreate) error
}

// GetAllRecords retrieve all records https://github.com/ionos-cloud/sdk-go-dns/blob/master/docs/api/RecordsApi.md#recordsget
func (c *DNSClient) GetAllRecords(ctx context.Context, offset int32) (sdk.RecordReadList, error) {
	log.Debugf("get all records with offset %d ...", offset)
	records, _, err := c.client.RecordsApi.RecordsGet(ctx).Limit(recordReadLimit).Offset(offset).Execute()
	if err != nil {
		log.Errorf("failed to get all records: %v", err)
		return records, err
	}
	if records.HasItems() {
		log.Debugf("found %d records", len(*records.Items))
	} else {
		log.Debug("no records found")
	}
	return records, err
}

// GetZoneRecords retrieve all records from zone
func (c *DNSClient) GetZoneRecords(ctx context.Context, zoneId string) (sdk.RecordReadList, error) {
	logger := log.WithField(logFieldZoneID, zoneId)
	logger.Debug("get records from zone")
	records, _, err := c.client.RecordsApi.ZonesRecordsGet(ctx, zoneId).Execute()
	if err != nil {
		logger.Errorf("failed to get records from zone: %v", err)
		return records, err
	}
	if records.HasItems() {
		logger.Debugf("found %d records", len(*records.Items))
	} else {
		logger.Debug("no records found")
	}
	return records, nil
}

func (c *DNSClient) GetRecordsByZoneIdAndName(ctx context.Context, zoneId, name string) (sdk.RecordReadList, error) {
	logger := log.WithField(logFieldZoneID, zoneId).WithField(logFieldRecordName, name)
	logger.Debug("get records from zone by name ...")
	records, _, err := c.client.RecordsApi.RecordsGet(ctx).FilterZoneId(zoneId).FilterName(name).
		FilterState(sdk.AVAILABLE).Execute()
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
	zones, _, err := c.client.ZonesApi.ZonesGet(ctx).Offset(offset).Limit(zoneReadLimit).Execute()
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

// GetZone client get zone method
func (c *DNSClient) GetZone(ctx context.Context, zoneId string) (sdk.ZoneRead, error) {
	logger := log.WithField(logFieldZoneID, zoneId)
	logger.Debugf("find zone by id: '%s' ...", zoneId)
	zone, _, err := c.client.ZonesApi.ZonesFindById(ctx, zoneId).Execute()
	if err != nil {
		logger.Errorf("failed to find zone: %v", err)
		return zone, err
	}
	logger.Debugf("zone found: %v", zone)
	return zone, err
}

// CreateRecord client create record method
func (c *DNSClient) CreateRecord(ctx context.Context, zoneId string, record sdk.RecordCreate) error {
	logger := log.WithField(logFieldZoneID, zoneId).WithField(logFieldRecordName, *record.GetProperties().GetName())
	logger.Debugf("creating record ...")
	if !c.dryRun {
		_, _, err := c.client.RecordsApi.ZonesRecordsPost(ctx, zoneId).RecordCreate(record).Execute()
		if err != nil {
			logger.Errorf("failed to create record: %v", err)
			return err
		}
		logger.Debug("record created successfully")
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
		_, err := c.client.RecordsApi.ZonesRecordsDelete(ctx, zoneId, recordId).Execute()
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

// Provider extends base provider to work with paas dns rest API
type Provider struct {
	provider.BaseProvider
	client       DNSService
	domainFilter endpoint.DomainFilter
}

// NewProvider returns an instance of new provider
func NewProvider(domainFilter endpoint.DomainFilter, configuration *ionos.Configuration, dryRun bool) *Provider {
	client := createClient(configuration)
	prov := &Provider{
		client:       &DNSClient{client: client, dryRun: dryRun},
		domainFilter: domainFilter,
	}
	return prov
}

func createClient(ionosConfig *ionos.Configuration) *sdk.APIClient {
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

	sdkConfig := sdk.NewConfiguration("", "", ionosConfig.APIKey, ionosConfig.APIEndpointURL)
	sdkConfig.Debug = ionosConfig.Debug
	apiClient := sdk.NewAPIClient(sdkConfig)
	return apiClient
}

func (p *Provider) readAllRecords(ctx context.Context) ([]sdk.RecordRead, error) {
	var result []sdk.RecordRead
	offset := int32(0)
	for {
		recordReadList, err := p.client.GetAllRecords(ctx, offset)
		if err != nil {
			return nil, err
		}
		if recordReadList.HasItems() {
			items := *recordReadList.GetItems()
			result = append(result, items...)
			offset += recordReadLimit
			if len(items) < recordReadLimit || offset >= recordReadMaxCount {
				break
			}
		} else {
			break
		}
	}

	if p.domainFilter.IsConfigured() {
		filteredResult := make([]sdk.RecordRead, 0)
		for _, record := range result {
			recordName := *record.GetProperties().GetName()
			if p.domainFilter.Match(recordName) {
				filteredResult = append(filteredResult, record)
			}
		}
		return filteredResult, nil
	} else {
		return result, nil
	}
}

func (p *Provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	allRecords, err := p.readAllRecords(ctx)
	if err != nil {
		return nil, err
	}
	epCollection := ionos.NewEndpointCollection[sdk.RecordRead](allRecords,
		func(recordRead sdk.RecordRead) *endpoint.Endpoint {
			record := *recordRead.GetProperties()
			return endpoint.NewEndpointWithTTL(*record.GetName(), *record.GetType(), endpoint.TTL(*record.GetTtl()), *record.GetContent())
		}, func(recordRead sdk.RecordRead) string {
			record := *recordRead.GetProperties()
			return *record.GetName() + "/" + *record.GetType() + "/" + strconv.Itoa(int(*record.GetTtl()))
		})
	return epCollection.RetrieveEndPoints(), nil
}

func (p *Provider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	epToCreate, epToDelete := ionos.GetCreateDeleteSetsFromChanges(changes)
	zt, err := p.createZoneTree(ctx)
	if err != nil {
		return err
	}
	recordsToDelete := ionos.NewRecordCollection[sdk.RecordRead](epToDelete, func(ep *endpoint.Endpoint) []sdk.RecordRead {
		logger := log.WithField(logFieldRecordName, ep.DNSName)
		records := make([]sdk.RecordRead, 0)
		zone := zt.FindZoneByDomainName(ep.DNSName)
		if zone.Id == nil {
			logger.Error("no zone found for record")
			return records
		}
		logger = logger.WithField(logFieldZoneID, *zone.GetId())
		zoneRecordReadList, err := p.client.GetRecordsByZoneIdAndName(ctx, *zone.GetId(), ep.DNSName)
		if err != nil {
			logger.Errorf("failed to get records for zone, error: %v", err)
			return records
		}
		if !zoneRecordReadList.HasItems() {
			logger.Warn("no records found to delete for zone")
			return records
		}
		result := make([]sdk.RecordRead, 0)
		for _, recordRead := range *zoneRecordReadList.GetItems() {
			record := *recordRead.GetProperties()
			if *record.GetType() == ep.RecordType {
				for _, target := range ep.Targets {
					if *record.GetContent() == target {
						result = append(result, recordRead)
					}
				}
			}
		}
		if len(result) == 0 {
			logger.Warnf("no records in zone fit to delete for endpoint: %v", ep)
		}
		return result
	})

	if err := recordsToDelete.ForEach(func(recordRead sdk.RecordRead) error {
		domainName := *recordRead.GetProperties().GetName()
		zone := zt.FindZoneByDomainName(domainName)
		err := p.client.DeleteRecord(ctx, *zone.GetId(), *recordRead.GetId())
		return err
	}); err != nil {
		return err
	}

	recordsToCreate := ionos.NewRecordCollection[*sdk.RecordCreate](epToCreate, func(ep *endpoint.Endpoint) []*sdk.RecordCreate {
		logger := log.WithField(logFieldRecordName, ep.DNSName)
		zone := zt.FindZoneByDomainName(ep.DNSName)
		if zone.Id == nil {
			logger.Warn("no zone found for record, skipping record creation")
			return nil
		}
		result := make([]*sdk.RecordCreate, 0)
		for _, target := range ep.Targets {
			record := sdk.NewRecord(ep.DNSName, ep.RecordType, target)
			ttl := int32(ep.RecordTTL)
			if ttl != 0 {
				record.SetTtl(ttl)
			}
			result = append(result, sdk.NewRecordCreate(*record))
		}
		return result
	})
	if err := recordsToCreate.ForEach(func(recordCreate *sdk.RecordCreate) error {
		domainName := *recordCreate.GetProperties().GetName()
		zone := zt.FindZoneByDomainName(domainName)
		err := p.client.CreateRecord(ctx, *zone.GetId(), *recordCreate)
		return err
	}); err != nil {
		return err
	}
	return nil
}

func (p *Provider) createZoneTree(ctx context.Context) (*ionos.ZoneTree[sdk.ZoneRead], error) {
	zt := ionos.NewZoneTree[sdk.ZoneRead]()
	var allZones []sdk.ZoneRead
	offset := int32(0)
	for {
		zoneReadList, err := p.client.GetZones(ctx, offset)
		if err != nil {
			return nil, err
		}
		if zoneReadList.HasItems() {
			items := *zoneReadList.GetItems()
			allZones = append(allZones, items...)
			offset += zoneReadLimit
			if len(items) < zoneReadLimit || offset >= zoneReadMaxCount {
				break
			}
		} else {
			break
		}
	}
	for _, zoneRead := range allZones {
		zoneName := *zoneRead.GetProperties().GetZoneName()
		if !p.domainFilter.IsConfigured() || p.domainFilter.Match(zoneName) {
			zt.AddZone(zoneRead, zoneName)
		}
	}
	return zt, nil
}
