package ionoscore

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	sdk "github.com/ionos-developer/dns-sdk-go"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

// Provider implements the DNS provider for IONOS DNS.
type Provider struct {
	provider.BaseProvider
	client       DnsService
	dryRun       bool
	domainFilter endpoint.DomainFilterInterface
	zoneIdToName map[string]string
}

var _ provider.Provider = (*Provider)(nil)

// NewProvider creates a new IONOS DNS provider.
func NewProvider(domanfilter endpoint.DomainFilter, client DnsService, isDryRun bool) (*Provider, error) {
	prov := &Provider{
		client:       client,
		dryRun:       isDryRun,
		domainFilter: domanfilter,
	}
	err := prov.setupZones(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to setup zones: %w", err)
	}
	if len(prov.zoneIdToName) == 0 {
		return nil, fmt.Errorf("no zones matching domain filter found")
	}

	return prov, nil
}

// Records returns the list of resource records in all zones.
func (p *Provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	var endpoints []*endpoint.Endpoint

	for zoneId, zoneName := range p.zoneIdToName {
		zoneInfo, err := p.client.GetZone(ctx, zoneId)
		if err != nil {
			return nil, fmt.Errorf("failed to get zone info for zone %s: %w", zoneName, err)
		}

		recordSets := map[string]*endpoint.Endpoint{}
		for _, r := range zoneInfo.Records {
			key := *r.Name + "/" + getType(r) + "/" + strconv.Itoa(int(*r.Ttl))
			if rrset, ok := recordSets[key]; ok {
				rrset.Targets = append(rrset.Targets, *r.Content)
			} else {
				recordSets[key] = recordToEndpoint(r)
			}
		}

		for _, ep := range recordSets {
			endpoints = append(endpoints, ep)
		}
	}
	log.Debugf("Records() found %d endpoints: %v", len(endpoints), endpoints)
	return endpoints, nil
}

// ApplyChanges applies a given set of changes.
func (p *Provider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	toCreate := make([]*endpoint.Endpoint, len(changes.Create))
	copy(toCreate, changes.Create)

	toDelete := make([]*endpoint.Endpoint, len(changes.Delete))
	copy(toDelete, changes.Delete)

	for i, updateOldEndpoint := range changes.UpdateOld {
		if !sameEndpoints(*updateOldEndpoint, *changes.UpdateNew[i]) {
			toDelete = append(toDelete, updateOldEndpoint)
			toCreate = append(toCreate, changes.UpdateNew[i])
		}
	}

	zonesToDeleteFrom := p.fetchZonesToDeleteFrom(ctx, toDelete)

	for _, e := range toDelete {
		zoneId := getHostZoneID(e.DNSName, p.zoneIdToName)
		if zoneId == "" {
			log.Warnf("No zone to delete %v from", e)
			continue
		}

		if zone, ok := zonesToDeleteFrom[zoneId]; ok {
			p.deleteEndpoint(ctx, e, zone)
		} else {
			log.Warnf("No zone to delete %v from", e)
		}
	}

	for _, e := range toCreate {
		p.createEndpoint(ctx, e, p.zoneIdToName)
	}

	return nil
}

// fetchZonesToDeleteFrom fetches all the zones that will be performed deletions upon.
func (p *Provider) fetchZonesToDeleteFrom(ctx context.Context, toDelete []*endpoint.Endpoint) map[string]*sdk.CustomerZone {
	zonesIdsToDeleteFrom := map[string]bool{}
	for _, e := range toDelete {
		zoneId := getHostZoneID(e.DNSName, p.zoneIdToName)
		if zoneId != "" {
			zonesIdsToDeleteFrom[zoneId] = true
		}
	}

	zonesToDeleteFrom := map[string]*sdk.CustomerZone{}
	for zoneId := range zonesIdsToDeleteFrom {
		zone, err := p.client.GetZone(ctx, zoneId)
		if err == nil {
			zonesToDeleteFrom[zoneId] = zone
		}
	}

	return zonesToDeleteFrom
}

// deleteEndpoint deletes all resource records for the endpoint through the IONOS DNS API.
func (p *Provider) deleteEndpoint(ctx context.Context, e *endpoint.Endpoint, zone *sdk.CustomerZone) {
	log.Infof("Delete endpoint %v", e)
	if p.dryRun {
		return
	}

	for _, target := range e.Targets {
		recordId := ""
		for _, record := range zone.Records {
			if *record.Name == e.DNSName && getType(record) == e.RecordType && *record.Content == target {
				recordId = *record.Id
				break
			}
		}

		if recordId == "" {
			log.Errorf("Record %v %v %v not found in zone", e.DNSName, e.RecordType, target)
			continue
		}

		if p.client.DeleteRecord(ctx, *zone.Id, recordId) != nil {
			log.Warnf("Failed to delete record %v %v %v", e.DNSName, e.RecordType, target)
		}
	}
}

// createEndpoint creates the record set for the endpoint using the IONOS DNS API.
func (p *Provider) createEndpoint(ctx context.Context, e *endpoint.Endpoint, zones map[string]string) {
	log.Infof("Create endpoint %v", e)
	if p.dryRun {
		return
	}

	zoneId := getHostZoneID(e.DNSName, zones)
	if zoneId == "" {
		log.Warnf("No zone to create %v into", e)
		return
	}

	records := endpointToRecords(e)
	if p.client.CreateRecords(ctx, zoneId, records) != nil {
		log.Warnf("Failed to create record for %v", e)
	}
}

// endpointToRecords converts an endpoint to a slice of records.
func endpointToRecords(endpoint *endpoint.Endpoint) []sdk.Record {
	records := make([]sdk.Record, 0)

	for _, target := range endpoint.Targets {
		record := sdk.NewRecord()

		record.SetName(endpoint.DNSName)
		record.SetType(sdk.RecordTypes(endpoint.RecordType))
		record.SetContent(target)

		ttl := int32(endpoint.RecordTTL)
		if ttl != 0 {
			record.SetTtl(ttl)
		}

		records = append(records, *record)
	}

	return records
}

// recordToEndpoint converts a record to an endpoint.
func recordToEndpoint(r sdk.RecordResponse) *endpoint.Endpoint {
	return endpoint.NewEndpointWithTTL(*r.Name, getType(r), endpoint.TTL(*r.Ttl), *r.Content)
}

// setupZones returns a ZoneID -> ZoneName mapping for zones that match domain filter.
func (p *Provider) setupZones(ctx context.Context) error {
	zones, err := p.client.GetZones(ctx)
	if err != nil {
		return err
	}

	mapping := map[string]string{}

	for _, zone := range zones {
		if p.GetDomainFilter().Match(*zone.Name) {
			mapping[*zone.Id] = *zone.Name
		}
	}

	p.zoneIdToName = mapping
	return nil
}

// getHostZoneID finds the best suitable DNS zone for the hostname.
func getHostZoneID(hostname string, zones map[string]string) string {
	longestZoneLength := 0
	resultID := ""

	for zoneID, zoneName := range zones {
		if !strings.HasSuffix(hostname, zoneName) {
			continue
		}
		ln := len(zoneName)
		if ln > longestZoneLength {
			resultID = zoneID
			longestZoneLength = ln
		}
	}

	return resultID
}

// getType returns the record type as string.
func getType(record sdk.RecordResponse) string {
	return string(*record.Type)
}

// sameEndpoints returns if the two endpoints have the same values.
func sameEndpoints(a endpoint.Endpoint, b endpoint.Endpoint) bool {
	return a.DNSName == b.DNSName && a.RecordType == b.RecordType && a.RecordTTL == b.RecordTTL && a.Targets.Same(b.Targets)
}

func (p *Provider) GetDomainFilter() endpoint.DomainFilterInterface {
	return p.domainFilter
}
