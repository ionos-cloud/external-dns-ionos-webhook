package ionoscore

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/ionos-developer/dns-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

// stubDnsService lets each test choose how every DnsService call behaves.
type stubDnsService struct {
	zoneErr   error
	createErr error
	deleteErr error

	created int
	deleted int
}

func (s *stubDnsService) GetZones(context.Context) ([]sdk.Zone, error) {
	z := sdk.NewZone()
	z.SetId("a")
	z.SetName("a.de")
	return []sdk.Zone{*z}, nil
}

func (s *stubDnsService) GetZone(_ context.Context, zoneId string) (*sdk.CustomerZone, error) {
	if s.zoneErr != nil {
		return nil, s.zoneErr
	}
	name := "a.de"
	zone := sdk.NewCustomerZone()
	zone.Id = &zoneId
	zone.Name = &name
	zone.Records = []sdk.RecordResponse{record(1, "www.a.de", sdk.A, "1.1.1.1", 300)}
	return zone, nil
}

func (s *stubDnsService) CreateRecords(context.Context, string, []sdk.Record) error {
	s.created++
	return s.createErr
}

func (s *stubDnsService) DeleteRecord(context.Context, string, string) error {
	s.deleted++
	return s.deleteErr
}

func newStubProvider(stub *stubDnsService, dryRun bool) *Provider {
	return &Provider{client: stub, dryRun: dryRun, domainFilter: endpoint.NewDomainFilter([]string{})}
}

func epFor(name, target string) *endpoint.Endpoint {
	return &endpoint.Endpoint{DNSName: name, RecordType: "A", RecordTTL: 300, Targets: endpoint.Targets{target}}
}

func TestRecordsSkipsZonesThatFail(t *testing.T) {
	p := newStubProvider(&stubDnsService{zoneErr: errors.New("boom")}, false)
	eps, err := p.Records(context.Background())
	require.NoError(t, err)
	assert.Empty(t, eps)
}

func TestApplyChangesEdgeCases(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name            string
		stub            *stubDnsService
		dryRun          bool
		changes         *plan.Changes
		expectedCreated int
		expectedDeleted int
	}{
		{
			name:    "delete: no zone for the endpoint",
			stub:    &stubDnsService{},
			changes: &plan.Changes{Delete: []*endpoint.Endpoint{epFor("www.other.org", "1.1.1.1")}},
		},
		{
			name:    "delete: zone cannot be fetched",
			stub:    &stubDnsService{zoneErr: errors.New("boom")},
			changes: &plan.Changes{Delete: []*endpoint.Endpoint{epFor("www.a.de", "1.1.1.1")}},
		},
		{
			name:    "delete: record not found in zone",
			stub:    &stubDnsService{},
			changes: &plan.Changes{Delete: []*endpoint.Endpoint{epFor("www.a.de", "9.9.9.9")}},
		},
		{
			name:            "delete: matching record",
			stub:            &stubDnsService{},
			changes:         &plan.Changes{Delete: []*endpoint.Endpoint{epFor("www.a.de", "1.1.1.1")}},
			expectedDeleted: 1,
		},
		{
			name:            "delete: API failure is tolerated",
			stub:            &stubDnsService{deleteErr: errors.New("boom")},
			changes:         &plan.Changes{Delete: []*endpoint.Endpoint{epFor("www.a.de", "1.1.1.1")}},
			expectedDeleted: 1,
		},
		{
			name:    "delete: dry run",
			stub:    &stubDnsService{},
			dryRun:  true,
			changes: &plan.Changes{Delete: []*endpoint.Endpoint{epFor("www.a.de", "1.1.1.1")}},
		},
		{
			name:    "create: no zone for the endpoint",
			stub:    &stubDnsService{},
			changes: &plan.Changes{Create: []*endpoint.Endpoint{epFor("www.other.org", "1.1.1.1")}},
		},
		{
			name:            "create: API failure is tolerated",
			stub:            &stubDnsService{createErr: errors.New("boom")},
			changes:         &plan.Changes{Create: []*endpoint.Endpoint{epFor("new.a.de", "1.1.1.1")}},
			expectedCreated: 1,
		},
		{
			name:    "create: dry run",
			stub:    &stubDnsService{},
			dryRun:  true,
			changes: &plan.Changes{Create: []*endpoint.Endpoint{epFor("new.a.de", "1.1.1.1")}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, newStubProvider(tc.stub, tc.dryRun).ApplyChanges(ctx, tc.changes))
			assert.Equal(t, tc.expectedCreated, tc.stub.created)
			assert.Equal(t, tc.expectedDeleted, tc.stub.deleted)
		})
	}
}
