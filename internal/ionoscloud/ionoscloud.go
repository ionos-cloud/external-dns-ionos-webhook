package ionoscloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionos"
	sdk "github.com/ionos-cloud/sdk-go-dns"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

const (
	logFieldZoneID        = "zoneID"
	logFieldRecordID      = "recordID"
	logFieldRecordName    = "recordName"
	logFieldRecordFQDN    = "recordFQDN"
	logFieldRecordType    = "recordType"
	logFieldRecordContent = "recordContent"
	logFieldRecordTTL     = "recordTTL"
	logFieldDomainFilter  = "domainFilter"
	// max number of records to read per request
	recordReadLimit = 1000
	// max number of records to read in total
	recordReadMaxCount = 10 * recordReadLimit
	// max number of zones to read per request
	zoneReadLimit = 1000
	// max number of zones to read in total
	zoneReadMaxCount = 10 * zoneReadLimit

	recordTypeSRV = "SRV"
	recordTypeMX  = "MX"
	recordTypeURI = "URI"
)

var _ provider.Provider = (*Provider)(nil)

// Provider extends base provider to work with paas dns rest API
type Provider struct {
	provider.BaseProvider
	client       DNSService
	domainFilter endpoint.DomainFilterInterface
	zoneTree     *ionos.ZoneTree[sdk.ZoneRead]
	zoneIdToName map[string]string
}

// NewProvider returns an instance of new provider
func NewProvider(domainFilter endpoint.DomainFilter, configuration *ionos.Configuration) (*Provider, error) {
	client := createClient(configuration)
	prov := &Provider{
		client:       &DNSClient{client: client, dryRun: configuration.DryRun},
		domainFilter: domainFilter,
	}
	err := prov.setupZones(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create zone tree: %w", err)
	}
	if len(prov.zoneIdToName) == 0 {
		return nil, fmt.Errorf("no zones matching domain filter found")
	}
	return prov, nil
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

	if ionosConfig.DryRun {
		log.Warnf("*** Dry run is enabled, no changes will be made to ionos cloud DNS ***")
	}

	sdkConfig := sdk.NewConfiguration("", "", ionosConfig.APIKey, ionosConfig.APIEndpointURL)
	sdkConfig.Debug = ionosConfig.Debug
	apiClient := sdk.NewAPIClient(sdkConfig)
	return apiClient
}

func (p *Provider) readAllRecords(ctx context.Context) ([]sdk.RecordRead, error) {
	var result []sdk.RecordRead
	offset := int32(0)
	getZoneRecords := func(zoneId string) error {
		for {
			recordReadList, err := p.client.GetZoneRecords(ctx, offset, zoneId)
			if err != nil {
				return err
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
		return nil
	}

	for id, name := range p.zoneIdToName {
		if err := getZoneRecords(id); err != nil {
			return nil, fmt.Errorf("failed to get records for zone %s, error: %w", name, err)
		}
	}
	return result, nil
}

func (p *Provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	allRecords, err := p.readAllRecords(ctx)
	if err != nil {
		return nil, err
	}
	epCollection := ionos.NewEndpointCollection[sdk.RecordRead](allRecords,
		func(recordRead sdk.RecordRead) *endpoint.Endpoint {
			recordProperties := *recordRead.GetProperties()
			recordMetadata := *recordRead.GetMetadata()
			target := *recordProperties.GetContent()
			priority, hasPriority := recordProperties.GetPriorityOk()
			recordType := *recordProperties.GetType()
			if (recordType == recordTypeSRV || recordType == recordTypeMX) && hasPriority {
				target = fmt.Sprintf("%d %s", *priority, target)
			}
			return endpoint.NewEndpointWithTTL(*recordMetadata.GetFqdn(), string(*recordProperties.GetType()),
				endpoint.TTL(*recordProperties.GetTtl()), target)
		}, func(recordRead sdk.RecordRead) string {
			recordProperties := *recordRead.GetProperties()
			recordMetadata := *recordRead.GetMetadata()
			return *recordMetadata.GetFqdn() + "/" + string(*recordProperties.GetType()) + "/" + strconv.Itoa(int(*recordProperties.GetTtl()))
		})
	return epCollection.RetrieveEndPoints(), nil
}

func (p *Provider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	epToCreate, epToDelete := ionos.GetCreateDeleteSetsFromChanges(changes)
	recordsToDelete := ionos.NewRecordCollection[sdk.RecordRead](epToDelete, func(ep *endpoint.Endpoint) []sdk.RecordRead {
		logger := log.WithField(logFieldRecordFQDN, ep.DNSName)
		records := make([]sdk.RecordRead, 0)
		zone := p.zoneTree.FindZoneByDomainName(ep.DNSName)
		if zone.Id == nil {
			logger.Error("no zone found for record")
			return records
		}
		logger = logger.WithField(logFieldZoneID, *zone.GetId())
		recordName := extractRecordName(ep.DNSName, zone)
		zoneRecordReadList, err := p.client.GetRecordsByZoneIdAndName(ctx, *zone.GetId(), recordName)
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
			if *record.GetType() == sdk.RecordType(ep.RecordType) {
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

	if err := recordsToDelete.ForEach(func(ep *endpoint.Endpoint, recordRead sdk.RecordRead) error {
		domainName := *recordRead.GetMetadata().GetFqdn()
		zone := p.zoneTree.FindZoneByDomainName(domainName)
		if !zone.HasId() {
			return fmt.Errorf("no zone found for domain '%s'", domainName)
		}
		err := p.client.DeleteRecord(ctx, *zone.GetId(), *recordRead.GetId())
		return err
	}); err != nil {
		return err
	}

	recordsToCreate := ionos.NewRecordCollection[*sdk.RecordCreate](epToCreate, func(ep *endpoint.Endpoint) []*sdk.RecordCreate {
		logger := log.WithField(logFieldRecordFQDN, ep.DNSName).WithField(logFieldRecordType, ep.RecordType)
		zone := p.zoneTree.FindZoneByDomainName(ep.DNSName)
		if !zone.HasId() {
			logger.Warnf("no zone found for domain '%s', skipping record creation", ep.DNSName)
			return nil
		}
		recordName := extractRecordName(ep.DNSName, zone)
		result := make([]*sdk.RecordCreate, 0)
		for _, target := range ep.Targets {
			content := target
			priority := int32(0)
			splitTarget := strings.Split(target, " ")
			if (ep.RecordType == recordTypeSRV || ep.RecordType == recordTypeMX ||
				ep.RecordType == recordTypeURI) && len(splitTarget) >= 2 {
				priority64, err := strconv.ParseInt(splitTarget[0], 10, 32)
				if err != nil {
					logger.Warnf("failed to parse priority from target '%s'", target)
				} else {
					priority = int32(priority64)
				}
				content = splitTarget[1]
				if ep.RecordType == recordTypeURI {
					content = target
				}
			}
			record := sdk.NewRecord(recordName, sdk.RecordType(ep.RecordType), content)
			ttl := int32(ep.RecordTTL)
			if ttl != 0 {
				record.SetTtl(ttl)
			}
			if priority != 0 {
				record.SetPriority(priority)
			}
			result = append(result, sdk.NewRecordCreate(*record))
		}
		return result
	})
	if err := recordsToCreate.ForEach(func(ep *endpoint.Endpoint, recordCreate *sdk.RecordCreate) error {
		zone := p.zoneTree.FindZoneByDomainName(ep.DNSName)
		if !zone.HasId() {
			return fmt.Errorf("no zone found for domain '%s'", ep.DNSName)
		}
		err := p.client.CreateRecord(ctx, *zone.GetId(), *recordCreate)
		return err
	}); err != nil {
		return err
	}
	return nil
}

func (p *Provider) setupZones(ctx context.Context) error {
	zt := ionos.NewZoneTree[sdk.ZoneRead]()
	idToName := make(map[string]string)
	var allZones []sdk.ZoneRead
	offset := int32(0)
	for {
		zoneReadList, err := p.client.GetZones(ctx, offset)
		if err != nil {
			return err
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
		if p.GetDomainFilter().Match(zoneName) {
			log.Debugf("zone %s matches domain filter", zoneName)
			zoneId := *zoneRead.GetProperties().GetZoneName()
			idToName[zoneId] = zoneName
			zt.AddZone(zoneRead, zoneName)
		}
	}

	p.zoneTree = zt
	p.zoneIdToName = idToName
	return nil
}

func extractRecordName(fqdn string, zone sdk.ZoneRead) string {
	zoneName := *zone.GetProperties().GetZoneName()
	partOfZoneName := strings.Index(fqdn, zoneName)
	if partOfZoneName == 0 {
		return ""
	}
	return fqdn[:partOfZoneName-1]
}

func (p *Provider) GetDomainFilter() endpoint.DomainFilterInterface {
	return p.domainFilter
}
