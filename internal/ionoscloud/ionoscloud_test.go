package ionoscloud

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"

	sdk "github.com/ionos-cloud/sdk-go-dns"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"

	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionos"
)

var (
	letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	testErr     = errors.New("test error")
)

func TestNewProvider(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	t.Setenv("IONOS_API_KEY", "1")
	domainFilter := endpoint.NewDomainFilter([]string{"a.de."})
	deZoneClient := &mockDNSClient{
		allZones: createZoneReadList(1, func(i int) (string, string) {
			return "deZoneId", "a.de"
		}),
	}
	t.Run("success, specific domain filter ", func(t *testing.T) {
		p, err := NewProvider(domainFilter, deZoneClient)
		require.NoError(t, err)
		require.True(t, true, p.GetDomainFilter().Match("a.de."))
		require.False(t, p.GetDomainFilter().Match("b.de."))
	})

	t.Run("success, no filtering", func(t *testing.T) {
		p, err := NewProvider(endpoint.DomainFilter{}, deZoneClient)
		require.NoError(t, err)
		require.True(t, true, p.GetDomainFilter().Match("everything.com"))
	})

	t.Run("error, no zones matching domain filter were found", func(t *testing.T) {
		noZonesClient := &mockDNSClient{
			allZones: createZoneReadList(0, nil),
		}
		p, err := NewProvider(domainFilter, noZonesClient)
		require.Error(t, err)
		require.Nil(t, p)
	})
}

func TestRecords(t *testing.T) {
	zoneIDa, zoneIDb := "zoneIDa", "zoneIDb"
	log.SetLevel(log.DebugLevel)
	ctx := context.Background()
	testCases := []struct {
		name               string
		givenRecordsByZone map[string]sdk.RecordReadList
		givenError         error
		expectedEndpoints  []*endpoint.Endpoint
		expectedError      error
	}{
		{
			name:               "no records",
			givenRecordsByZone: map[string]sdk.RecordReadList{},
			expectedEndpoints:  []*endpoint.Endpoint{},
		},
		{
			name:               "error reading records",
			givenRecordsByZone: map[string]sdk.RecordReadList{},
			givenError:         testErr,
			expectedEndpoints:  []*endpoint.Endpoint{},
			expectedError:      testErr,
		},
		{
			name: "multiple A records",
			givenRecordsByZone: map[string]sdk.RecordReadList{
				zoneIDa: createZoneRecordsReadList(3, 0, 0, func(i int) (string, string, string, int32, string) {
					recordName := "a" + fmt.Sprintf("%d", i+1)
					fqdn := recordName + ".a.de"
					return recordName, fqdn, "A", int32((i + 1) * 100), fmt.Sprintf("%d.%d.%d.%d", i+1, i+1, i+1, i+1)
				}),
			},
			expectedEndpoints: createEndpointSlice(3, func(i int) (string, string, endpoint.TTL, []string) {
				return "a" + fmt.Sprintf("%d", i+1) + ".a.de", "A", endpoint.TTL((i + 1) * 100), []string{fmt.Sprintf("%d.%d.%d.%d", i+1, i+1, i+1, i+1)}
			}),
		},
		{
			name: "records of Type A and SRV",
			givenRecordsByZone: map[string]sdk.RecordReadList{
				zoneIDa: createZoneRecordsReadList(1, 0, 0, func(i int) (string, string, string, int32, string) {
					return "a", "a.de", "A", 100, "1.1.1.1"
				}),
				zoneIDb: createZoneRecordsReadList(1, 0, 333, func(i int) (string, string, string, int32, string) {
					return "b", "b.de", "SRV", 200, "server.example.com"
				}),
			},
			expectedEndpoints: createEndpointSlice(2, func(i int) (string, string, endpoint.TTL, []string) {
				if i == 0 {
					return "a.de", "A", 100, []string{"1.1.1.1"}
				}
				return "b.de", "SRV", 200, []string{"333 server.example.com"}
			}),
		},
		{
			name: "records of Type A and MX",
			givenRecordsByZone: map[string]sdk.RecordReadList{
				zoneIDa: createZoneRecordsReadList(1, 0, 333, func(i int) (string, string, string, int32, string) {
					return "a", "a.de", "A", 100, "1.1.1.1"
				}),
				zoneIDb: createZoneRecordsReadList(1, 0, 333, func(i int) (string, string, string, int32, string) {
					return "b", "b.de", "MX", 200, "server.example.com"
				}),
			},
			expectedEndpoints: createEndpointSlice(2, func(i int) (string, string, endpoint.TTL, []string) {
				if i == 0 {
					return "a.de", "A", 100, []string{"1.1.1.1"}
				}
				return "b.de", "MX", 200, []string{"333 server.example.com"}
			}),
		},
		{
			name: "records of Type A and URI",
			givenRecordsByZone: map[string]sdk.RecordReadList{
				zoneIDa: createZoneRecordsReadList(1, 0, 333, func(i int) (string, string, string, int32, string) {
					return "a", "a.de", "A", 100, "1.1.1.1"
				}),
				zoneIDb: createZoneRecordsReadList(1, 0, 333, func(i int) (string, string, string, int32, string) {
					return "b", "b.de", "URI", 200, "333 333 server.example.com"
				}),
			},
			expectedEndpoints: createEndpointSlice(2, func(i int) (string, string, endpoint.TTL, []string) {
				if i == 0 {
					return "a.de", "A", 100, []string{"1.1.1.1"}
				}
				return "b.de", "URI", 200, []string{"333 333 server.example.com"}
			}),
		},
		{
			name: "multiple records",
			givenRecordsByZone: map[string]sdk.RecordReadList{
				zoneIDa: createZoneRecordsReadList(3, 0, 0, func(i int) (string, string, string, int32, string) {
					recordName := "a" + fmt.Sprintf("%d", i+1)
					fqdn := recordName + ".a.de"
					return recordName, fqdn, "A", int32((i + 1) * 100), fmt.Sprintf("%d.%d.%d.%d", i+1, i+1, i+1, i+1)
				}),
				zoneIDb: createZoneRecordsReadList(3, 3, 0, func(i int) (string, string, string, int32, string) {
					recordName := "b" + fmt.Sprintf("%d", i+4)
					fqdn := recordName + ".b.de"
					return recordName, fqdn, "A", int32((i + 4) * 100), fmt.Sprintf("%d.%d.%d.%d", i+4, i+4, i+4, i+4)
				}),
			},
			expectedEndpoints: createEndpointSlice(6, func(i int) (string, string, endpoint.TTL, []string) {
				if i < 3 {
					return "a" + fmt.Sprintf("%d", i+1) + ".a.de", "A", endpoint.TTL((i + 1) * 100), []string{fmt.Sprintf("%d.%d.%d.%d", i+1, i+1, i+1, i+1)}
				}
				return "b" + fmt.Sprintf("%d", i+1) + ".b.de", "A", endpoint.TTL((i + 1) * 100), []string{fmt.Sprintf("%d.%d.%d.%d", i+1,
					i+1, i+1, i+1)}
			}),
		},
		{
			name: "records mapped to same endpoint",
			givenRecordsByZone: map[string]sdk.RecordReadList{
				zoneIDa: createZoneRecordsReadList(2, 0, 0, func(i int) (string, string, string, int32, string) {
					return "", "a.de", "A", int32(300), fmt.Sprintf("%d.%d.%d.%d", i+1, i+1, i+1, i+1)
				}),
				zoneIDb: createZoneRecordsReadList(1, 2, 0, func(i int) (string, string, string, int32, string) {
					return "", "c.de", "A", int32(300), fmt.Sprintf("%d.%d.%d.%d", i+3, i+3, i+3, i+3)
				}),
			},
			expectedEndpoints: createEndpointSlice(2, func(i int) (string, string, endpoint.TTL, []string) {
				if i == 0 {
					return "a.de", "A", endpoint.TTL(300), []string{"1.1.1.1", "2.2.2.2"}
				} else {
					return "c.de", "A", endpoint.TTL(300), []string{"3.3.3.3"}
				}
			}),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDnsClient := &mockDNSClient{
				returnError: tc.givenError,
				zoneRecords: tc.givenRecordsByZone,
			}
			prov := &Provider{
				client: mockDnsClient,
				zoneIdToName: map[string]string{
					zoneIDa: "a.de",
					zoneIDb: "b.de",
				},
			}
			endpoints, err := prov.Records(ctx)
			if tc.expectedError != nil {
				require.ErrorIs(t, err, tc.expectedError)
				return
			}
			require.NoError(t, err)
			require.Len(t, endpoints, len(tc.expectedEndpoints))
			assert.ElementsMatch(t, tc.expectedEndpoints, endpoints)
		})
	}
}

func TestApplyChanges(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	log.SetReportCaller(true)
	deZoneId, comZoneId := "deZoneId", "comZoneId"
	// TODO move zone tree creation somewhere else
	zt := ionos.NewZoneTree[sdk.ZoneRead]()
	zt.AddZone(sdk.ZoneRead{
		Id: &deZoneId,
		Properties: &sdk.Zone{
			ZoneName: ptr("a.de"),
		},
	}, "a.de")
	zt.AddZone(sdk.ZoneRead{
		Id: &comZoneId,
		Properties: &sdk.Zone{
			ZoneName: ptr("a.com"),
		},
	}, "a.com")

	ctx := context.Background()
	testCases := []struct {
		name                   string
		givenZoneRecords       map[string]sdk.RecordReadList
		givenError             error
		whenChanges            *plan.Changes
		expectedError          error
		expectedRecordsCreated map[string][]sdk.RecordCreate
		expectedRecordsDeleted map[string][]string
	}{
		{
			name:                   "no changes",
			givenZoneRecords:       map[string]sdk.RecordReadList{},
			whenChanges:            &plan.Changes{},
			expectedRecordsCreated: nil,
			expectedRecordsDeleted: nil,
		},
		{
			name: "error applying changes",
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "A", endpoint.TTL(300), []string{"1.2.3.4"}
				}),
			},
			givenError:    testErr,
			expectedError: testErr,
		},
		{
			name: "create one record in a blank zone",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "A", endpoint.TTL(300), []string{"1.2.3.4"}
				}),
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: createRecordCreateSlice(1, func(i int) (string, string, int32, string, int32) {
					return "", "A", int32(300), "1.2.3.4", 0
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "create a SRV record in a blank zone",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "SRV", endpoint.TTL(500), []string{"777 myHost.de"}
				}),
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: createRecordCreateSlice(1, func(i int) (string, string, int32, string, int32) {
					return "", "SRV", int32(500), "myHost.de", 777
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "create a SRV record with no priority field in target",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "SRV", endpoint.TTL(700), []string{"myHost.de"}
				}),
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: createRecordCreateSlice(1, func(i int) (string, string, int32, string, int32) {
					return "", "SRV", int32(700), "myHost.de", 0
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "create a SRV record with wrong priority syntax in target",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "SRV", endpoint.TTL(900), []string{"NaN myHost.de"}
				}),
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: createRecordCreateSlice(1, func(i int) (string, string, int32, string, int32) {
					return "", "SRV", int32(900), "myHost.de", 0
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "create a MX record in a blank zone",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "MX", endpoint.TTL(500), []string{"777 myHost.de"}
				}),
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: createRecordCreateSlice(1, func(i int) (string, string, int32, string, int32) {
					return "", "MX", int32(500), "myHost.de", 777
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "create a MX record with no priority field in target",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "MX", endpoint.TTL(700), []string{"myHost.de"}
				}),
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: createRecordCreateSlice(1, func(i int) (string, string, int32, string, int32) {
					return "", "MX", int32(700), "myHost.de", 0
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "create a MX record with wrong priority syntax in target",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "MX", endpoint.TTL(900), []string{"NaN myHost.de"}
				}),
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: createRecordCreateSlice(1, func(i int) (string, string, int32, string, int32) {
					return "", "MX", int32(900), "myHost.de", 0
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "create a URI record in a blank zone",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "URI", endpoint.TTL(500), []string{"777 777 myHost.de"}
				}),
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: createRecordCreateSlice(1, func(i int) (string, string, int32, string, int32) {
					return "", "URI", int32(500), "777 777 myHost.de", 777
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "create a URI record with wrong priority syntax in target",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "URI", endpoint.TTL(900), []string{"NaN 777 myHost.de"}
				}),
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: createRecordCreateSlice(1, func(i int) (string, string, int32, string, int32) {
					return "", "URI", int32(900), "NaN 777 myHost.de", 0
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "create a record which is filtered out from the domain filter",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "d.de", "A", endpoint.TTL(300), []string{"1.2.3.4"}
				}),
			},
			expectedRecordsCreated: nil,
			expectedRecordsDeleted: nil,
		},
		{
			name: "create 2 records from one endpoint in a blank zone",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Create: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "A", endpoint.TTL(300), []string{"1.2.3.4", "5.6.7.8"}
				}),
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: createRecordCreateSlice(2, func(i int) (string, string, int32, string, int32) {
					if i == 0 {
						return "a", "A", int32(300), "1.2.3.4", 0
					} else {
						return "a", "A", int32(300), "5.6.7.8", 0
					}
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "delete the only record in a zone",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(1, 0, 0, func(i int) (string, string, string, int32, string) {
					return "a", "a.de", "A", int32(300), "1.2.3.4"
				}),
			},
			whenChanges: &plan.Changes{
				Delete: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "A", endpoint.TTL(300), []string{"1.2.3.4"}
				}),
			},
			expectedRecordsDeleted: map[string][]string{
				deZoneId: {"0"},
			},
		},
		{
			name: "delete a record which is filtered out from the domain filter",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(1, 0, 0, func(i int) (string, string, string, int32, string) {
					return "d", "d.de", "A", int32(300), "1.2.3.4"
				}),
			},
			whenChanges: &plan.Changes{
				Delete: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "d.de", "A", endpoint.TTL(300), []string{"1.2.3.4"}
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "delete multiple records, in different zones",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(2, 0, 0, func(n int) (string, string, string, int32, string) {
					if n == 0 {
						return "a", "a.de", "A", 300, "1.2.3.4"
					} else {
						return "a", "a.de", "A", 300, "5.6.7.8"
					}
				}),
				comZoneId: createZoneRecordsReadList(1, 2, 0, func(n int) (string, string, string, int32, string) {
					return "a", "a.com", "A", 300, "11.22.33.44"
				}),
			},
			whenChanges: &plan.Changes{
				Delete: createEndpointSlice(2, func(i int) (string, string, endpoint.TTL, []string) {
					if i == 0 {
						return "a.de", "A", endpoint.TTL(300), []string{"1.2.3.4", "5.6.7.8"}
					} else {
						return "a.com", "A", endpoint.TTL(300), []string{"11.22.33.44"}
					}
				}),
			},
			expectedRecordsDeleted: map[string][]string{
				deZoneId:  {"0", "1"},
				comZoneId: {"2"},
			},
		},
		{
			name: "delete record which is not in the zone, deletes nothing",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(0, 0, 0, nil),
			},
			whenChanges: &plan.Changes{
				Delete: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "A", endpoint.TTL(300), []string{"1.2.3.4"}
				}),
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "delete one record from targets part of endpoint",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(1, 0, 0, func(i int) (string, string, string, int32, string) {
					return "a", "a.de", "A", 300, "1.2.3.4"
				}),
			},
			whenChanges: &plan.Changes{
				Delete: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "A", endpoint.TTL(300), []string{"1.2.3.4", "5.6.7.8"}
				}),
			},
			expectedRecordsDeleted: map[string][]string{
				deZoneId: {"0"},
			},
		},
		{
			name: "update single record",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(1, 0, 0, func(i int) (string, string, string, int32, string) {
					return "a", "a.de", "A", 300, "1.2.3.4"
				}),
			},
			whenChanges: &plan.Changes{
				UpdateOld: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "A", endpoint.TTL(300), []string{"1.2.3.4"}
				}),
				UpdateNew: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "A", endpoint.TTL(300), []string{"5.6.7.8"}
				}),
			},
			expectedRecordsDeleted: map[string][]string{
				deZoneId: {"0"},
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: createRecordCreateSlice(1, func(i int) (string, string, int32, string, int32) {
					return "a", "A", 300, "5.6.7.8", 0
				}),
			},
		},
		{
			name: "update a record which is filtered out by domain filter, does nothing",
			whenChanges: &plan.Changes{
				UpdateOld: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "d.de", "A", endpoint.TTL(300), []string{"1.2.3.4"}
				}),
				UpdateNew: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "d.de", "A", endpoint.TTL(300), []string{"5.6.7.8"}
				}),
			},
			expectedRecordsDeleted: nil,
			expectedRecordsCreated: nil,
		},
		{
			name: "update when old and new endpoint are the same, does nothing",
			givenZoneRecords: map[string]sdk.RecordReadList{
				deZoneId: createZoneRecordsReadList(1, 0, 0, func(i int) (string, string, string, int32, string) {
					return "a", "a.de", "A", 300, "1.2.3.4"
				}),
			},
			whenChanges: &plan.Changes{
				UpdateOld: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "A", endpoint.TTL(300), []string{"1.2.3.4"}
				}),
				UpdateNew: createEndpointSlice(1, func(i int) (string, string, endpoint.TTL, []string) {
					return "a.de", "A", endpoint.TTL(300), []string{"1.2.3.4"}
				}),
			},
			expectedRecordsDeleted: nil,
			expectedRecordsCreated: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDnsClient := &mockDNSClient{
				zoneRecords: tc.givenZoneRecords,
				returnError: tc.givenError,
			}
			prov := &Provider{
				client: mockDnsClient,
				zoneIdToName: map[string]string{
					deZoneId:  "a.de",
					comZoneId: "a.com",
				},
				zoneTree: zt,
			}
			err := prov.ApplyChanges(ctx, tc.whenChanges)
			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, mockDnsClient.createdRecords, len(tc.expectedRecordsCreated))
			for zoneId, expectedRecordsCreated := range tc.expectedRecordsCreated {
				actualRecords, ok := mockDnsClient.createdRecords[zoneId]
				require.True(t, ok)
				for i, actualRecord := range actualRecords {
					expJson, _ := expectedRecordsCreated[i].MarshalJSON()
					actJson, _ := actualRecord.MarshalJSON()
					require.Equal(t, expJson, actJson)
				}
			}
			for zoneId, expectedDeletedRecordIds := range tc.expectedRecordsDeleted {
				require.Len(t, mockDnsClient.deletedRecords[zoneId], len(expectedDeletedRecordIds), "deleted records in zone '%s' do not fit", zoneId)
				actualDeletedRecordIds, ok := mockDnsClient.deletedRecords[zoneId]
				require.True(t, ok)
				assert.ElementsMatch(t, expectedDeletedRecordIds, actualDeletedRecordIds)
			}
		})
	}
}

func TestAdjustEndpoints(t *testing.T) {
	prov := &Provider{}
	endpoints := createEndpointSlice(rand.Intn(5), func(i int) (string, string, endpoint.TTL, []string) {
		return RandStringRunes(10), RandStringRunes(1), endpoint.TTL(300), []string{RandStringRunes(5)}
	})
	actualEndpoints, err := prov.AdjustEndpoints(endpoints)
	require.NoError(t, err)
	require.Equal(t, endpoints, actualEndpoints)
}

func TestReadMaxRecords(t *testing.T) {
	prov := &Provider{
		domainFilter: endpoint.DomainFilter{},
		client:       pagingMockDNSService{t: t},
		zoneIdToName: map[string]string{"zoneId": "zoneName"},
	}
	endpoints, err := prov.Records(context.Background())
	require.NoError(t, err)
	require.Len(t, endpoints, recordReadMaxCount)
}

func TestReadMaxZones(t *testing.T) {
	prov := &Provider{domainFilter: endpoint.DomainFilter{}, client: pagingMockDNSService{t: t}}
	err := prov.setupZones(context.Background())
	require.NoError(t, err)
	require.Equal(t, zoneReadMaxCount, prov.zoneTree.GetZonesCount())
}

type pagingMockDNSService struct {
	t *testing.T
}

func (p pagingMockDNSService) GetZoneRecords(ctx context.Context, offset int32, zoneId string) (sdk.RecordReadList, error) {
	require.Equal(p.t, 0, int(offset)%recordReadLimit)
	records := createZoneRecordsReadList(recordReadLimit, int(offset), 0, func(i int) (string, string, string, int32, string) {
		recordName := fmt.Sprintf("a%d", int(offset)+i)
		return recordName, recordName + ".de", "A", 300, "1.1.1.1"
	})
	return records, nil
}

func (pagingMockDNSService) GetRecordsByZoneIdAndName(ctx context.Context, zoneId, name string) (sdk.RecordReadList, error) {
	panic("implement me")
}

func (p pagingMockDNSService) GetZones(ctx context.Context, offset int32) (sdk.ZoneReadList, error) {
	require.Equal(p.t, 0, int(offset)%zoneReadLimit)
	zones := createZoneReadList(zoneReadLimit, func(i int) (string, string) {
		idStr := fmt.Sprintf("%d", int(offset)+i)
		return idStr, fmt.Sprintf("zone%s.de", idStr)
	})
	return zones, nil
}

func (pagingMockDNSService) GetZone(ctx context.Context, zoneId string) (sdk.ZoneRead, error) {
	panic("implement me")
}

func (pagingMockDNSService) DeleteRecord(ctx context.Context, zoneId string, recordId string) error {
	panic("implement me")
}

func (pagingMockDNSService) CreateRecord(ctx context.Context, zoneId string, record sdk.RecordCreate) error {
	panic("implement me")
}

type mockDNSClient struct {
	returnError    error
	zoneRecords    map[string]sdk.RecordReadList
	allZones       sdk.ZoneReadList
	createdRecords map[string][]sdk.RecordCreate // zoneId -> recordCreates
	deletedRecords map[string][]string           // zoneId -> recordIds
}

func (c *mockDNSClient) GetZoneRecords(ctx context.Context, offset int32, zoneId string) (sdk.RecordReadList, error) {
	log.Debugf("GetZoneRecords called with zoneId %s", zoneId)
	return c.zoneRecords[zoneId], c.returnError
}

func (c *mockDNSClient) GetRecordsByZoneIdAndName(ctx context.Context, zoneId, name string) (sdk.RecordReadList, error) {
	log.Debugf("GetRecordsByZoneIdAndName called with zoneId %s and name %s", zoneId, name)
	result := make([]sdk.RecordRead, 0)
	recordsOfZone := c.zoneRecords[zoneId]
	for _, recordRead := range *recordsOfZone.GetItems() {
		if *recordRead.GetProperties().GetName() == name {
			result = append(result, recordRead)
		}
	}
	return sdk.RecordReadList{Items: &result}, c.returnError
}

func (c *mockDNSClient) GetZones(ctx context.Context, offset int32) (sdk.ZoneReadList, error) {
	log.Debug("GetZones called ")
	if c.allZones.HasItems() {
		for _, zone := range *c.allZones.GetItems() {
			log.Debugf("GetZones: zone '%s' with id '%s'", *zone.GetProperties().GetZoneName(), *zone.GetId())
		}
	} else {
		log.Debug("GetZones: no zones")
	}
	return c.allZones, c.returnError
}

func (c *mockDNSClient) GetZone(ctx context.Context, zoneId string) (sdk.ZoneRead, error) {
	log.Debugf("GetZone called with zoneId '%s", zoneId)
	for _, zone := range *c.allZones.GetItems() {
		if *zone.GetId() == zoneId {
			return zone, nil
		}
	}
	return *sdk.NewZoneReadWithDefaults(), c.returnError
}

func (c *mockDNSClient) CreateRecord(ctx context.Context, zoneId string, record sdk.RecordCreate) error {
	log.Debugf("CreateRecord called with zoneId %s and record %v", zoneId, record)
	if c.createdRecords == nil {
		c.createdRecords = make(map[string][]sdk.RecordCreate)
	}
	c.createdRecords[zoneId] = append(c.createdRecords[zoneId], record)
	return c.returnError
}

func (c *mockDNSClient) DeleteRecord(ctx context.Context, zoneId string, recordId string) error {
	log.Debugf("DeleteRecord called with zoneId %s and recordId %s", zoneId, recordId)
	if c.deletedRecords == nil {
		c.deletedRecords = make(map[string][]string)
	}
	c.deletedRecords[zoneId] = append(c.deletedRecords[zoneId], recordId)
	return c.returnError
}

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func createZoneReadList(count int, modifier func(int) (string, string)) sdk.ZoneReadList {
	zones := make([]sdk.ZoneRead, count)
	for i := 0; i < count; i++ {
		id, name := modifier(i)
		zones[i] = sdk.ZoneRead{
			Id: sdk.PtrString(id),
			Properties: &sdk.Zone{
				ZoneName: sdk.PtrString(name),
				Enabled:  sdk.PtrBool(true),
			},
		}
	}
	return sdk.ZoneReadList{Items: &zones}
}

func createRecordCreateSlice(count int, modifier func(int) (string, string, int32, string, int32)) []sdk.RecordCreate {
	records := make([]sdk.RecordCreate, count)
	for i := 0; i < count; i++ {
		name, typ, ttl, content, prio := modifier(i)
		records[i] = sdk.RecordCreate{
			Properties: &sdk.Record{
				Name:    sdk.PtrString(name),
				Type:    sdk.RecordType(typ).Ptr(),
				Ttl:     sdk.PtrInt32(ttl),
				Content: sdk.PtrString(content),
				Enabled: sdk.PtrBool(true),
			},
		}
		if prio != 0 {
			records[i].Properties.SetPriority(prio)
		}
	}
	return records
}

func createZoneRecordsReadList(count, idOffset int, priority int32, modifier func(int) (string, string, string, int32, string)) sdk.RecordReadList {
	records := make([]sdk.RecordRead, count)
	for i := 0; i < count; i++ {
		name, fqdn, typ, ttl, content := modifier(i)
		// use random number as id
		id := i + idOffset
		records[i] = sdk.RecordRead{
			Id: sdk.PtrString(fmt.Sprintf("%d", id)),
			Properties: &sdk.Record{
				Name:     sdk.PtrString(name),
				Type:     sdk.RecordType(typ).Ptr(),
				Ttl:      sdk.PtrInt32(ttl),
				Content:  sdk.PtrString(content),
				Priority: sdk.PtrInt32(priority),
			},
			Metadata: &sdk.MetadataWithStateFqdnZoneId{
				Fqdn: sdk.PtrString(fqdn),
			},
		}
	}
	return sdk.RecordReadList{Items: &records}
}

func createEndpointSlice(count int, modifier func(int) (string, string, endpoint.TTL, []string)) []*endpoint.Endpoint {
	endpoints := make([]*endpoint.Endpoint, count)
	for i := 0; i < count; i++ {
		name, typ, ttl, targets := modifier(i)
		endpoints[i] = &endpoint.Endpoint{
			DNSName:    name,
			RecordType: typ,
			Targets:    targets,
			RecordTTL:  ttl,
			Labels:     map[string]string{},
		}
	}
	return endpoints
}

func ptr[T any](v T) *T {
	return &v
}
