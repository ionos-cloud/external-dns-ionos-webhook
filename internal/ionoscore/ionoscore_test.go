package ionoscore

import (
	"context"
	"fmt"
	"sort"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"

	sdk "github.com/ionos-developer/dns-sdk-go"
	"github.com/stretchr/testify/require"
)

var zoneIdToZoneName = map[string]string{
	"a": "a.de",
	"b": "b.de",
}

type mockDnsService struct {
	testErrorReturned bool
	createdRecords    map[string][]sdk.Record
	deletedRecords    map[string][]string
}

func TestNewProvider(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	successClient := &mockDnsService{testErrorReturned: false}
	errorClient := &mockDnsService{testErrorReturned: true}

	t.Run("success with specific domain filter", func(t *testing.T) {
		domainFilter := endpoint.NewDomainFilter([]string{"a.de."})
		p, err := NewProvider(domainFilter, successClient, true)
		require.NoError(t, err)
		assert.Equal(t, true, p.dryRun)
		assert.Equal(t, p.zoneIdToName, map[string]string{"a": "a.de"})
		assert.True(t, p.GetDomainFilter().Match("a.de"))
		assert.False(t, p.GetDomainFilter().Match("ab.de"))
		assert.NotNilf(t, p.client, "client should not be nil")
	})

	t.Run("success with everything allowed domain filter", func(t *testing.T) {
		p, err := NewProvider(endpoint.DomainFilter{}, successClient, false)
		require.NoError(t, err)
		assert.Equal(t, p.zoneIdToName, map[string]string{"a": "a.de", "b": "b.de"})
		assert.Equal(t, false, p.dryRun)
		assert.True(t, p.GetDomainFilter().Match("everything"))
		assert.NotNilf(t, p.client, "client should not be nil")
	})

	t.Run("error when getting zones", func(t *testing.T) {
		domainFilter := endpoint.NewDomainFilter([]string{"a.de."})
		p, err := NewProvider(domainFilter, errorClient, true)
		require.Error(t, err)
		assert.Nil(t, p)
	})
}

func TestRecords(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		p := &Provider{
			client: &mockDnsService{testErrorReturned: false},
			zoneIdToName: map[string]string{
				"a": "a.de",
				"b": "b.de",
			},
		}
		endpoints, err := p.Records(ctx)
		require.NoError(t, err, "should not fail")
		assert.Equal(t, 5, len(endpoints))
	})

	t.Run("error when getting records", func(t *testing.T) {
		p := &Provider{
			client: &mockDnsService{testErrorReturned: true},
			zoneIdToName: map[string]string{
				"a": "a.de",
				"b": "b.de",
			},
		}
		endpoints, err := p.Records(ctx)
		require.Nil(t, endpoints)
		require.Error(t, err, "should fail")
	})
}

func TestApplyChanges(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mockClient := &mockDnsService{testErrorReturned: false}
		p := &Provider{
			client: mockClient,
			zoneIdToName: map[string]string{
				"a": "a.de",
				"b": "b.de",
			},
		}
		err := p.ApplyChanges(ctx, changes())
		require.NoError(t, err)

		// 3 records must be deleted
		require.Equal(t, mockClient.deletedRecords["b"], []string{"6"})
		sort.Strings(mockClient.deletedRecords["a"])
		require.Equal(t, mockClient.deletedRecords["a"], []string{"1", "2"})
		// 3 records must be created
		assert.True(t, mockClient.isRecordCreated("a", "a.de", sdk.A, "3.3.3.3", 2000), "Record a.de A 3.3.3.3 not created")
		assert.True(t, mockClient.isRecordCreated("a", "a.de", sdk.A, "4.4.4.4", 2000), "Record a.de A 4.4.4.4 not created")
		assert.True(t, mockClient.isRecordCreated("a", "new.a.de", sdk.CNAME, "a.de", 0), "Record new.a.de CNAME a.de not created")
	})

	t.Run("deletion failed ", func(t *testing.T) {
		mockClient := &mockDnsService{testErrorReturned: true}
		p := &Provider{
			client: mockClient,
			zoneIdToName: map[string]string{
				"b": "b.de",
			},
		}
		err := p.ApplyChanges(ctx, changes())
		require.NoError(t, err)
		require.Len(t, mockClient.deletedRecords["b"], 0)
	})
}

func (m *mockDnsService) GetZones(ctx context.Context) ([]sdk.Zone, error) {
	if m.testErrorReturned {
		return nil, fmt.Errorf("GetZones failed")
	}

	a := sdk.NewZone()
	a.SetId("a")
	a.SetName("a.de")

	b := sdk.NewZone()
	b.SetId("b")
	b.SetName("b.de")

	return []sdk.Zone{*a, *b}, nil
}

func (m *mockDnsService) GetZone(ctx context.Context, zoneId string) (*sdk.CustomerZone, error) {
	if m.testErrorReturned {
		return nil, fmt.Errorf("GetZone failed")
	}

	zoneName := zoneIdToZoneName[zoneId]
	zone := sdk.NewCustomerZone()
	zone.Id = &zoneId
	zone.Name = &zoneName
	if zoneName == "a.de" {
		zone.Records = []sdk.RecordResponse{
			record(1, "a.de", sdk.A, "1.1.1.1", 1000),
			record(2, "a.de", sdk.A, "2.2.2.2", 1000),
			record(3, "cname.a.de", sdk.CNAME, "cname.de", 1000),
			record(4, "aaaa.a.de", sdk.AAAA, "1::", 1000),
			record(5, "aaaa.a.de", sdk.AAAA, "2::", 2000),
		}
	} else {
		zone.Records = []sdk.RecordResponse{record(6, "b.de", sdk.A, "5.5.5.5", 1000)}
	}

	return zone, nil
}

func (m *mockDnsService) CreateRecords(ctx context.Context, zoneId string, records []sdk.Record) error {
	if m.testErrorReturned {
		return fmt.Errorf("CreateRecords failed")
	}
	if m.createdRecords == nil {
		m.createdRecords = make(map[string][]sdk.Record)
	}
	m.createdRecords[zoneId] = append(m.createdRecords[zoneId], records...)
	return nil
}

func (m *mockDnsService) DeleteRecord(ctx context.Context, zoneId string, recordId string) error {
	if m.testErrorReturned {
		return fmt.Errorf("DeleteRecord failed")
	}
	if m.deletedRecords == nil {
		m.deletedRecords = make(map[string][]string)
	}
	m.deletedRecords[zoneId] = append(m.deletedRecords[zoneId], recordId)
	return nil
}

func record(id int, name string, recordType sdk.RecordTypes, content string, ttl int32) sdk.RecordResponse {
	r := sdk.NewRecordResponse()
	idStr := fmt.Sprint(id)
	r.Id = &idStr
	r.Name = &name
	r.Type = &recordType
	r.Content = &content
	r.Ttl = &ttl
	return *r
}

func changes() *plan.Changes {
	changes := &plan.Changes{}

	changes.Create = []*endpoint.Endpoint{
		{DNSName: "new.a.de", Targets: endpoint.Targets{"a.de"}, RecordType: "CNAME"},
	}
	changes.Delete = []*endpoint.Endpoint{{DNSName: "b.de", RecordType: "A", Targets: endpoint.Targets{"5.5.5.5"}}}
	changes.UpdateOld = []*endpoint.Endpoint{{DNSName: "a.de", RecordType: "A", Targets: endpoint.Targets{"1.1.1.1", "2.2.2.2"}, RecordTTL: 1000}}
	changes.UpdateNew = []*endpoint.Endpoint{{DNSName: "a.de", RecordType: "A", Targets: endpoint.Targets{"3.3.3.3", "4.4.4.4"}, RecordTTL: 2000}}

	return changes
}

func (m mockDnsService) isRecordCreated(zoneId string, name string, recordType sdk.RecordTypes, content string, ttl int32) bool {
	for _, record := range m.createdRecords[zoneId] {
		if *record.Name == name && *record.Type == recordType && *record.Content == content && (ttl == 0 || *record.Ttl == ttl) {
			return true
		}
	}

	return false
}
