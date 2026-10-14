package ionoscloud

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/ionos-cloud/sdk-go-dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

// stubDNSService lets each test choose how every DNSService call behaves.
type stubDNSService struct {
	zones         sdk.ZoneReadList
	zonesErr      error
	recordsByName sdk.RecordReadList
	recordsErr    error
	createErr     error
	deleteErr     error

	created int
	deleted int
}

func (s *stubDNSService) GetAllRecords(context.Context, int32) (sdk.RecordReadList, error) {
	return sdk.RecordReadList{}, nil
}

func (s *stubDNSService) GetRecordsByZoneIdAndName(context.Context, string, string) (sdk.RecordReadList, error) {
	return s.recordsByName, s.recordsErr
}

func (s *stubDNSService) GetZones(context.Context, int32) (sdk.ZoneReadList, error) {
	return s.zones, s.zonesErr
}

func (s *stubDNSService) DeleteRecord(context.Context, string, string) error {
	s.deleted++
	return s.deleteErr
}

func (s *stubDNSService) CreateRecord(context.Context, string, sdk.RecordCreate) error {
	s.created++
	return s.createErr
}

func oneZone() sdk.ZoneReadList {
	return createZoneReadList(1, func(int) (string, string) { return "zoneId", "a.de" })
}

func storedRecord(fqdn, typ, content string) sdk.RecordReadList {
	return createRecordReadList(1, 0, 0, func(int) (string, string, string, int32, string) {
		return "www", fqdn, typ, 300, content
	})
}

func deleteChanges() *plan.Changes {
	return &plan.Changes{Delete: []*endpoint.Endpoint{
		{DNSName: "www.a.de", RecordType: "A", Targets: endpoint.Targets{"1.1.1.1"}},
	}}
}

func createChanges() *plan.Changes {
	return &plan.Changes{Create: []*endpoint.Endpoint{
		{DNSName: "www.a.de", RecordType: "A", RecordTTL: 300, Targets: endpoint.Targets{"1.1.1.1"}},
	}}
}

func TestApplyChangesEdgeCases(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name            string
		stub            *stubDNSService
		changes         *plan.Changes
		expectedErr     string
		expectedCreated int
		expectedDeleted int
	}{
		{
			name:        "zones cannot be listed",
			stub:        &stubDNSService{zonesErr: errors.New("zones boom")},
			changes:     createChanges(),
			expectedErr: "zones boom",
		},
		{
			name:    "delete: no zone for endpoint is skipped",
			stub:    &stubDNSService{},
			changes: deleteChanges(),
		},
		{
			name:    "delete: lookup error is skipped",
			stub:    &stubDNSService{zones: oneZone(), recordsErr: errors.New("lookup boom")},
			changes: deleteChanges(),
		},
		{
			name:    "delete: no stored records is skipped",
			stub:    &stubDNSService{zones: oneZone()},
			changes: deleteChanges(),
		},
		{
			name:    "delete: stored records do not match the target",
			stub:    &stubDNSService{zones: oneZone(), recordsByName: storedRecord("www.a.de", "A", "9.9.9.9")},
			changes: deleteChanges(),
		},
		{
			name:            "delete: matching record is deleted",
			stub:            &stubDNSService{zones: oneZone(), recordsByName: storedRecord("www.a.de", "A", "1.1.1.1")},
			changes:         deleteChanges(),
			expectedDeleted: 1,
		},
		{
			name:            "delete: DeleteRecord fails",
			stub:            &stubDNSService{zones: oneZone(), recordsByName: storedRecord("www.a.de", "A", "1.1.1.1"), deleteErr: errors.New("delete boom")},
			changes:         deleteChanges(),
			expectedErr:     "delete boom",
			expectedDeleted: 1,
		},
		{
			name:        "delete: stored record belongs to an unknown zone",
			stub:        &stubDNSService{zones: oneZone(), recordsByName: storedRecord("www.other.org", "A", "1.1.1.1")},
			changes:     deleteChanges(),
			expectedErr: "no zone found for domain 'www.other.org'",
		},
		{
			name:    "create: no zone for endpoint is skipped",
			stub:    &stubDNSService{},
			changes: createChanges(),
		},
		{
			name:            "create: CreateRecord fails",
			stub:            &stubDNSService{zones: oneZone(), createErr: errors.New("create boom")},
			changes:         createChanges(),
			expectedErr:     "create boom",
			expectedCreated: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &Provider{client: tc.stub, domainFilter: endpoint.NewDomainFilter([]string{})}
			err := p.ApplyChanges(ctx, tc.changes)
			if tc.expectedErr != "" {
				require.ErrorContains(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.expectedCreated, tc.stub.created)
			assert.Equal(t, tc.expectedDeleted, tc.stub.deleted)
		})
	}
}

func TestCreateZoneTreeEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("no zones", func(t *testing.T) {
		p := &Provider{client: &stubDNSService{}, domainFilter: endpoint.NewDomainFilter([]string{})}
		zt, err := p.createZoneTree(ctx)
		require.NoError(t, err)
		zone := zt.FindZoneByDomainName("www.a.de")
		assert.False(t, zone.HasId())
	})
	t.Run("zones outside the domain filter are ignored", func(t *testing.T) {
		p := &Provider{client: &stubDNSService{zones: oneZone()}, domainFilter: endpoint.NewDomainFilter([]string{"b.de"})}
		zt, err := p.createZoneTree(ctx)
		require.NoError(t, err)
		zone := zt.FindZoneByDomainName("www.a.de")
		assert.False(t, zone.HasId())
	})
}
