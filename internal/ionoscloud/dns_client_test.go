package ionoscloud

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionos"
	sdk "github.com/ionos-cloud/sdk-go-dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type requestRecorder struct {
	mu       sync.Mutex
	requests []string
}

func (r *requestRecorder) add(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req.Method+" "+req.URL.Path)
}

func (r *requestRecorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}

// newTestDNSClient returns a DNSClient backed by a fake IONOS Cloud DNS API.
// Every request is answered with the given status and body.
func newTestDNSClient(t *testing.T, dryRun bool, status int, body string) (*DNSClient, *requestRecorder) {
	t.Helper()
	rec := &requestRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	cfg := &ionos.Configuration{APIKey: "token", APIEndpointURL: srv.URL, DryRun: dryRun}
	return &DNSClient{clientFactory: newClientFactory(cfg), dryRun: dryRun}, rec
}

func newFailingFactoryDNSClient() *DNSClient {
	return &DNSClient{
		clientFactory: &clientFactory{
			tokenProvider: &mockTokenProvider{err: errors.New("token error")},
			ionosConfig:   &ionos.Configuration{},
		},
	}
}

func testRecordCreate() sdk.RecordCreate {
	return sdk.RecordCreate{Properties: &sdk.Record{
		Name:    sdk.PtrString("www"),
		Type:    sdk.RecordType("A").Ptr(),
		Content: sdk.PtrString("1.1.1.1"),
		Ttl:     sdk.PtrInt32(300),
	}}
}

const (
	recordsBody = `{"items":[{"id":"r1","metadata":{"fqdn":"www.a.de"},"properties":{"name":"www","type":"A","content":"1.1.1.1","ttl":300}}]}`
	zonesBody   = `{"items":[{"id":"z1","properties":{"zoneName":"a.de"}}]}`
)

func TestDNSClientGetAllRecords(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		c, rec := newTestDNSClient(t, false, http.StatusOK, recordsBody)
		records, err := c.GetAllRecords(ctx, 0)
		require.NoError(t, err)
		require.True(t, records.HasItems())
		assert.Len(t, *records.Items, 1)
		assert.Equal(t, []string{"GET /records"}, rec.all())
	})
	t.Run("no items", func(t *testing.T) {
		c, _ := newTestDNSClient(t, false, http.StatusOK, `{}`)
		records, err := c.GetAllRecords(ctx, 0)
		require.NoError(t, err)
		assert.False(t, records.HasItems())
	})
	t.Run("api error", func(t *testing.T) {
		c, _ := newTestDNSClient(t, false, http.StatusInternalServerError, `{}`)
		_, err := c.GetAllRecords(ctx, 0)
		assert.Error(t, err)
	})
	t.Run("client factory error", func(t *testing.T) {
		_, err := newFailingFactoryDNSClient().GetAllRecords(ctx, 0)
		assert.ErrorContains(t, err, "failed to create IONOS Cloud DNS client")
	})
}

func TestDNSClientGetRecordsByZoneIdAndName(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		c, rec := newTestDNSClient(t, false, http.StatusOK, recordsBody)
		records, err := c.GetRecordsByZoneIdAndName(ctx, "z1", "www")
		require.NoError(t, err)
		require.True(t, records.HasItems())
		assert.Equal(t, []string{"GET /records"}, rec.all())
	})
	t.Run("no items", func(t *testing.T) {
		c, _ := newTestDNSClient(t, false, http.StatusOK, `{}`)
		records, err := c.GetRecordsByZoneIdAndName(ctx, "z1", "www")
		require.NoError(t, err)
		assert.False(t, records.HasItems())
	})
	t.Run("api error", func(t *testing.T) {
		c, _ := newTestDNSClient(t, false, http.StatusInternalServerError, `{}`)
		_, err := c.GetRecordsByZoneIdAndName(ctx, "z1", "www")
		assert.Error(t, err)
	})
	t.Run("client factory error", func(t *testing.T) {
		_, err := newFailingFactoryDNSClient().GetRecordsByZoneIdAndName(ctx, "z1", "www")
		assert.ErrorContains(t, err, "failed to create IONOS Cloud DNS client")
	})
}

func TestDNSClientGetZones(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		c, rec := newTestDNSClient(t, false, http.StatusOK, zonesBody)
		zones, err := c.GetZones(ctx, 0)
		require.NoError(t, err)
		require.True(t, zones.HasItems())
		assert.Equal(t, []string{"GET /zones"}, rec.all())
	})
	t.Run("no items", func(t *testing.T) {
		c, _ := newTestDNSClient(t, false, http.StatusOK, `{}`)
		zones, err := c.GetZones(ctx, 0)
		require.NoError(t, err)
		assert.False(t, zones.HasItems())
	})
	t.Run("api error", func(t *testing.T) {
		c, _ := newTestDNSClient(t, false, http.StatusInternalServerError, `{}`)
		_, err := c.GetZones(ctx, 0)
		assert.Error(t, err)
	})
	t.Run("client factory error", func(t *testing.T) {
		_, err := newFailingFactoryDNSClient().GetZones(ctx, 0)
		assert.ErrorContains(t, err, "failed to create IONOS Cloud DNS client")
	})
}

func TestDNSClientCreateRecord(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		c, rec := newTestDNSClient(t, false, http.StatusAccepted, `{"id":"r1"}`)
		require.NoError(t, c.CreateRecord(ctx, "z1", testRecordCreate()))
		assert.Equal(t, []string{"POST /zones/z1/records"}, rec.all())
	})
	t.Run("dry run", func(t *testing.T) {
		c, rec := newTestDNSClient(t, true, http.StatusAccepted, `{"id":"r1"}`)
		require.NoError(t, c.CreateRecord(ctx, "z1", testRecordCreate()))
		assert.Empty(t, rec.all())
	})
	t.Run("api error", func(t *testing.T) {
		c, _ := newTestDNSClient(t, false, http.StatusInternalServerError, `{}`)
		assert.Error(t, c.CreateRecord(ctx, "z1", testRecordCreate()))
	})
	t.Run("client factory error", func(t *testing.T) {
		err := newFailingFactoryDNSClient().CreateRecord(ctx, "z1", testRecordCreate())
		assert.ErrorContains(t, err, "failed to create IONOS Cloud DNS client")
	})
}

func TestDNSClientDeleteRecord(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		c, rec := newTestDNSClient(t, false, http.StatusAccepted, `{}`)
		require.NoError(t, c.DeleteRecord(ctx, "z1", "r1"))
		assert.Equal(t, []string{"DELETE /zones/z1/records/r1"}, rec.all())
	})
	t.Run("dry run", func(t *testing.T) {
		c, rec := newTestDNSClient(t, true, http.StatusAccepted, `{}`)
		require.NoError(t, c.DeleteRecord(ctx, "z1", "r1"))
		assert.Empty(t, rec.all())
	})
	t.Run("api error", func(t *testing.T) {
		c, _ := newTestDNSClient(t, false, http.StatusInternalServerError, `{}`)
		assert.Error(t, c.DeleteRecord(ctx, "z1", "r1"))
	})
	t.Run("client factory error", func(t *testing.T) {
		err := newFailingFactoryDNSClient().DeleteRecord(ctx, "z1", "r1")
		assert.ErrorContains(t, err, "failed to create IONOS Cloud DNS client")
	})
}

func TestCreateClientFactory(t *testing.T) {
	jwt := "eyJhbGciOiJub25lIn0.eyJzdWIiOiJ4In0.sig"
	for name, cfg := range map[string]*ionos.Configuration{
		"jwt api key":       {APIKey: jwt, DryRun: true},
		"opaque api key":    {APIKey: "not-a-jwt"},
		"username/password": {Username: "u", Password: strings.Repeat("p", 3)},
	} {
		t.Run(name, func(t *testing.T) {
			cf := createClientFactory(cfg)
			require.NotNil(t, cf)
			assert.Same(t, cfg, cf.ionosConfig)
		})
	}
}
