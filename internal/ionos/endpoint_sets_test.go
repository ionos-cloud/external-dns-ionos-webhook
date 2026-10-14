package ionos

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

func ep(name, typ string, ttl endpoint.TTL, targets ...string) *endpoint.Endpoint {
	return &endpoint.Endpoint{DNSName: name, RecordType: typ, RecordTTL: ttl, Targets: targets}
}

func TestGetCreateDeleteSetsFromChanges(t *testing.T) {
	t.Run("create and delete are passed through", func(t *testing.T) {
		create, del := ep("c.a.de", "A", 300, "1.1.1.1"), ep("d.a.de", "A", 300, "2.2.2.2")
		toCreate, toDelete := GetCreateDeleteSetsFromChanges(&plan.Changes{
			Create: []*endpoint.Endpoint{create},
			Delete: []*endpoint.Endpoint{del},
		})
		assert.Equal(t, []*endpoint.Endpoint{create}, toCreate)
		assert.Equal(t, []*endpoint.Endpoint{del}, toDelete)
	})

	t.Run("an update becomes delete of the old and create of the new", func(t *testing.T) {
		oldEp, newEp := ep("u.a.de", "A", 300, "1.1.1.1"), ep("u.a.de", "A", 300, "3.3.3.3")
		toCreate, toDelete := GetCreateDeleteSetsFromChanges(&plan.Changes{
			UpdateOld: []*endpoint.Endpoint{oldEp},
			UpdateNew: []*endpoint.Endpoint{newEp},
		})
		assert.Equal(t, []*endpoint.Endpoint{newEp}, toCreate)
		assert.Equal(t, []*endpoint.Endpoint{oldEp}, toDelete)
	})

	t.Run("an update without difference is ignored", func(t *testing.T) {
		toCreate, toDelete := GetCreateDeleteSetsFromChanges(&plan.Changes{
			UpdateOld: []*endpoint.Endpoint{ep("u.a.de", "A", 300, "1.1.1.1")},
			UpdateNew: []*endpoint.Endpoint{ep("u.a.de", "A", 300, "1.1.1.1")},
		})
		assert.Empty(t, toCreate)
		assert.Empty(t, toDelete)
	})

	t.Run("the input slices are not modified", func(t *testing.T) {
		changes := &plan.Changes{
			Create:    []*endpoint.Endpoint{ep("c.a.de", "A", 300, "1.1.1.1")},
			UpdateOld: []*endpoint.Endpoint{ep("u.a.de", "A", 300, "1.1.1.1")},
			UpdateNew: []*endpoint.Endpoint{ep("u.a.de", "A", 600, "1.1.1.1")},
		}
		GetCreateDeleteSetsFromChanges(changes)
		assert.Len(t, changes.Create, 1)
		assert.Empty(t, changes.Delete)
	})
}

func TestEndpointsAreDifferent(t *testing.T) {
	base := ep("a.de", "A", 300, "1.1.1.1")
	for name, tc := range map[string]struct {
		other    *endpoint.Endpoint
		expected bool
	}{
		"identical":         {ep("a.de", "A", 300, "1.1.1.1"), false},
		"other name":        {ep("b.de", "A", 300, "1.1.1.1"), true},
		"other type":        {ep("a.de", "TXT", 300, "1.1.1.1"), true},
		"other ttl":         {ep("a.de", "A", 600, "1.1.1.1"), true},
		"other target":      {ep("a.de", "A", 300, "2.2.2.2"), true},
		"extra target":      {ep("a.de", "A", 300, "1.1.1.1", "2.2.2.2"), true},
		"targets reordered": {ep("a.de", "A", 300, "1.1.1.1"), false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, endpointsAreDifferent(*base, *tc.other))
		})
	}
}

func TestRecordCollectionForEach(t *testing.T) {
	a, b := ep("a.de", "A", 300, "1.1.1.1"), ep("b.de", "A", 300, "2.2.2.2")
	rc := NewRecordCollection[string]([]*endpoint.Endpoint{a, b}, func(e *endpoint.Endpoint) []string {
		return []string{e.DNSName + "-1", e.DNSName + "-2"}
	})

	t.Run("visits every record of every endpoint", func(t *testing.T) {
		visited := map[string]*endpoint.Endpoint{}
		require.NoError(t, rc.ForEach(func(e *endpoint.Endpoint, r string) error {
			visited[r] = e
			return nil
		}))
		assert.Equal(t, map[string]*endpoint.Endpoint{
			"a.de-1": a, "a.de-2": a, "b.de-1": b, "b.de-2": b,
		}, visited)
	})

	t.Run("stops at the first error", func(t *testing.T) {
		boom := errors.New("boom")
		calls := 0
		err := rc.ForEach(func(*endpoint.Endpoint, string) error {
			calls++
			return boom
		})
		assert.ErrorIs(t, err, boom)
		assert.Equal(t, 1, calls)
	})

	t.Run("endpoint without records is fine", func(t *testing.T) {
		empty := NewRecordCollection[string]([]*endpoint.Endpoint{a}, func(*endpoint.Endpoint) []string { return nil })
		assert.NoError(t, empty.ForEach(func(*endpoint.Endpoint, string) error {
			t.Fatal("must not be called")
			return nil
		}))
	})
}

func TestZoneTreeCount(t *testing.T) {
	zt := NewZoneTree[string]()
	assert.Equal(t, 0, zt.GetZonesCount())
	zt.AddZone("z1", "a.de")
	zt.AddZone("z2", "sub.a.de")
	assert.Equal(t, 2, zt.GetZonesCount())
	assert.Equal(t, "z2", zt.FindZoneByDomainName("www.sub.a.de"))
	assert.Equal(t, "z1", zt.FindZoneByDomainName("www.a.de"))
}
