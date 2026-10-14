package ionoscore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionos"
	sdk "github.com/ionos-developer/dns-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDnsClient returns a DnsClient backed by a fake IONOS core DNS API that
// answers every request with the given status and body and records "METHOD path".
func newTestDnsClient(t *testing.T, status int, body string) (DnsClient, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	client := createClient(&ionos.Configuration{APIKey: "key", AuthHeader: "X-API-Key", APIEndpointURL: srv.URL})
	return DnsClient{client: client}, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), requests...)
	}
}

func TestDnsClientGetZones(t *testing.T) {
	ctx := context.Background()
	c, reqs := newTestDnsClient(t, http.StatusOK, `[{"id":"z1","name":"a.de","type":"NATIVE"}]`)
	zones, err := c.GetZones(ctx)
	require.NoError(t, err)
	require.Len(t, zones, 1)
	assert.Equal(t, "z1", *zones[0].Id)
	assert.Equal(t, []string{"GET /v1/zones"}, reqs())

	c, _ = newTestDnsClient(t, http.StatusInternalServerError, `{}`)
	_, err = c.GetZones(ctx)
	assert.Error(t, err)
}

func TestDnsClientGetZone(t *testing.T) {
	ctx := context.Background()
	c, reqs := newTestDnsClient(t, http.StatusOK, `{"id":"z1","name":"a.de","records":[]}`)
	zone, err := c.GetZone(ctx, "z1")
	require.NoError(t, err)
	assert.Equal(t, "z1", *zone.Id)
	assert.Equal(t, []string{"GET /v1/zones/z1"}, reqs())

	c, _ = newTestDnsClient(t, http.StatusInternalServerError, `{}`)
	_, err = c.GetZone(ctx, "z1")
	assert.Error(t, err)
}

func TestDnsClientCreateRecords(t *testing.T) {
	ctx := context.Background()
	name, content, typ := "www", "1.1.1.1", sdk.RecordTypes("A")
	records := []sdk.Record{{Name: &name, Content: &content, Type: &typ}}

	c, reqs := newTestDnsClient(t, http.StatusCreated, `[]`)
	require.NoError(t, c.CreateRecords(ctx, "z1", records))
	assert.Equal(t, []string{"POST /v1/zones/z1/records"}, reqs())

	c, _ = newTestDnsClient(t, http.StatusInternalServerError, `{}`)
	assert.Error(t, c.CreateRecords(ctx, "z1", records))
}

func TestDnsClientDeleteRecord(t *testing.T) {
	ctx := context.Background()
	c, reqs := newTestDnsClient(t, http.StatusOK, `{}`)
	require.NoError(t, c.DeleteRecord(ctx, "z1", "r1"))
	assert.Equal(t, []string{"DELETE /v1/zones/z1/records/r1"}, reqs())

	c, _ = newTestDnsClient(t, http.StatusInternalServerError, `{}`)
	assert.Error(t, c.DeleteRecord(ctx, "z1", "r1"))
}

func TestCreateClientMasksShortKeys(t *testing.T) {
	for _, key := range []string{"", "ab", "abcdef"} {
		assert.NotNil(t, createClient(&ionos.Configuration{APIKey: key, DryRun: true}))
	}
}
