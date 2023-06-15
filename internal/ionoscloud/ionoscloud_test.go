package ionoscloud

import (
	"context"
	"fmt"
	"github.com/ionos-cloud/external-dns-ionos-plugin/internal/ionos"
	"github.com/ionos-cloud/external-dns-ionos-plugin/pkg/endpoint"
	"github.com/ionos-cloud/external-dns-ionos-plugin/pkg/plan"
	sdk "github.com/ionos-cloud/sdk-go-dns"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func createRecordReadList(count int, modifier func(int) (string, string, int32, string)) sdk.RecordReadList {
	records := make([]sdk.RecordRead, count)
	for i := 0; i < count; i++ {
		name, typ, ttl, content := modifier(i)
		records[i] = sdk.RecordRead{
			Id: sdk.PtrString(fmt.Sprintf("%d", i+1)),
			Properties: &sdk.Record{
				Name:    sdk.PtrString(name),
				Type:    sdk.PtrString(typ),
				Ttl:     sdk.PtrInt32(ttl),
				Content: sdk.PtrString(content),
			},
		}
	}
	return sdk.RecordReadList{Items: &records}
}

//func createRecordReadList(count int, name, typ string) sdk.RecordReadList {
//	records := make([]sdk.RecordRead, count)
//	for i := 0; i < count; i++ {
//		n := strconv.Itoa(i + 1)
//		records[i] = sdk.RecordRead{
//			Id: sdk.PtrString(fmt.Sprintf("%d", i)),
//			Properties: &sdk.Record{
//				Name:    sdk.PtrString("a" + n + "." + name),
//				Type:    sdk.PtrString(typ),
//				Ttl:     sdk.PtrInt32(int32((i + 1) * 100)),
//				Content: sdk.PtrString(n + "." + n + "." + n + "." + n),
//			},
//		}
//	}
//	return sdk.RecordReadList{Items: &records}
//}

func TestNewProvider(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	t.Setenv("IONOS_API_KEY", "1")

	p := NewProvider(endpoint.NewDomainFilter([]string{"a.de."}), &ionos.Configuration{}, true)
	require.Equal(t, true, p.domainFilter.IsConfigured())
	require.Equal(t, false, p.domainFilter.Match("b.de."))

	p = NewProvider(endpoint.DomainFilter{}, &ionos.Configuration{}, false)
	require.Equal(t, false, p.domainFilter.IsConfigured())
	require.Equal(t, true, p.domainFilter.Match("a.de."))
}

func TestRecords(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	ctx := context.Background()
	testCases := []struct {
		name              string
		givenRecords      sdk.RecordReadList
		givenError        error
		expectedEndpoints []*endpoint.Endpoint
		expectedError     error
	}{
		{
			name:              "no records",
			givenRecords:      sdk.RecordReadList{},
			expectedEndpoints: []*endpoint.Endpoint{},
		},
		{
			name: "multiple A records",
			givenRecords: createRecordReadList(3, func(i int) (string, string, int32, string) {
				return "a" + fmt.Sprintf("%d", i+1) + ".a.de", "A", int32((i + 1) * 100), fmt.Sprintf("%d.%d.%d.%d", i+1, i+1, i+1, i+1)
			}),
			expectedEndpoints: []*endpoint.Endpoint{
				{
					DNSName:    "a1.a.de",
					RecordType: "A",
					Targets:    []string{"1.1.1.1"},
					RecordTTL:  100,
					Labels:     map[string]string{},
				},
				{
					DNSName:    "a2.a.de",
					RecordType: "A",
					Targets:    []string{"2.2.2.2"},
					RecordTTL:  200,
					Labels:     map[string]string{},
				},
				{
					DNSName:    "a3.a.de",
					RecordType: "A",
					Targets:    []string{"3.3.3.3"},
					RecordTTL:  300,
					Labels:     map[string]string{},
				},
			},
		},
		{
			name: "records mapped to same endpoint",
			givenRecords: sdk.RecordReadList{
				Items: &[]sdk.RecordRead{
					{
						Id: sdk.PtrString("1"),
						Properties: &sdk.Record{
							Name:    sdk.PtrString("a.de"),
							Type:    sdk.PtrString("A"),
							Ttl:     sdk.PtrInt32(300),
							Content: sdk.PtrString("1.1.1.1"),
						},
					},
					{
						Id: sdk.PtrString("2"),
						Properties: &sdk.Record{
							Name:    sdk.PtrString("a.de"),
							Type:    sdk.PtrString("A"),
							Ttl:     sdk.PtrInt32(300),
							Content: sdk.PtrString("2.2.2.2"),
						},
					},
					{
						Id: sdk.PtrString("3"),
						Properties: &sdk.Record{
							Name:    sdk.PtrString("c.de"),
							Type:    sdk.PtrString("A"),
							Ttl:     sdk.PtrInt32(300),
							Content: sdk.PtrString("3.3.3.3"),
						},
					},
				},
			},
			expectedEndpoints: []*endpoint.Endpoint{
				{
					DNSName:    "a.de",
					RecordType: "A",
					Targets:    []string{"1.1.1.1", "2.2.2.2"},
					RecordTTL:  300,
					Labels:     map[string]string{},
				},
				{
					DNSName:    "c.de",
					RecordType: "A",
					Targets:    []string{"3.3.3.3"},
					RecordTTL:  300,
					Labels:     map[string]string{},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDnsClient := &mockDNSClient{
				allRecords:  tc.givenRecords,
				returnError: tc.givenError,
			}
			provider := &Provider{client: mockDnsClient}
			endpoints, err := provider.Records(ctx)
			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, endpoints, len(tc.expectedEndpoints))
			assert.ElementsMatch(t, tc.expectedEndpoints, endpoints)
		})
	}

	mockDnsClient := &mockDNSClient{
		allRecords: *sdk.NewRecordReadListWithDefaults(),
	}

	provider := &Provider{client: mockDnsClient}
	endpoints, err := provider.Records(ctx)
	require.NoError(t, err)
	require.Len(t, endpoints, 0)
}

func TestApplyChanges(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	log.SetReportCaller(true)
	deZoneId := "deZoneId"
	comZoneId := "comZoneId"
	ctx := context.Background()
	testCases := []struct {
		name                   string
		givenRecords           sdk.RecordReadList
		givenZones             sdk.ZoneReadList
		givenZoneRecords       map[string]*sdk.RecordReadList
		givenError             error
		whenChanges            *plan.Changes
		expectedError          error
		expectedRecordsCreated map[string][]sdk.RecordCreate
		expectedRecordsDeleted map[string][]string
	}{
		{
			name:                   "no changes",
			givenZones:             sdk.ZoneReadList{},
			givenZoneRecords:       map[string]*sdk.RecordReadList{},
			whenChanges:            &plan.Changes{},
			expectedRecordsCreated: nil,
			expectedRecordsDeleted: nil,
		},
		{
			name: "create one record in a blank zone",
			givenZones: sdk.ZoneReadList{
				Items: &[]sdk.ZoneRead{
					{
						Id: sdk.PtrString(deZoneId),
						Properties: &sdk.Zone{
							ZoneName: sdk.PtrString("de"),
						},
					},
				},
			},
			givenZoneRecords: map[string]*sdk.RecordReadList{
				deZoneId: {
					Items: &[]sdk.RecordRead{},
				},
			},
			whenChanges: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{
						DNSName:    "a.de",
						RecordType: "A",
						Targets:    []string{"1.2.3.4"},
						RecordTTL:  300,
						Labels:     map[string]string{},
					},
				},
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: {
					{
						Properties: &sdk.Record{
							Name:    sdk.PtrString("a.de"),
							Type:    sdk.PtrString("A"),
							Ttl:     sdk.PtrInt32(300),
							Content: sdk.PtrString("1.2.3.4"),
							Enabled: sdk.PtrBool(true),
						},
					},
				},
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "create 2 records from one endpoint in a blank zone",
			givenZones: sdk.ZoneReadList{
				Items: &[]sdk.ZoneRead{
					{
						Id: sdk.PtrString(deZoneId),
						Properties: &sdk.Zone{
							ZoneName: sdk.PtrString("de"),
						},
					},
				},
			},
			givenZoneRecords: map[string]*sdk.RecordReadList{
				deZoneId: {
					Items: &[]sdk.RecordRead{},
				},
			},
			whenChanges: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{
						DNSName:    "a.de",
						RecordType: "A",
						Targets:    []string{"1.2.3.4", "5.6.7.8"},
						RecordTTL:  300,
						Labels:     map[string]string{},
					},
				},
			},
			expectedRecordsCreated: map[string][]sdk.RecordCreate{
				deZoneId: {
					{
						Properties: &sdk.Record{
							Name:    sdk.PtrString("a.de"),
							Type:    sdk.PtrString("A"),
							Ttl:     sdk.PtrInt32(300),
							Content: sdk.PtrString("1.2.3.4"),
							Enabled: sdk.PtrBool(true),
						},
					},
					{
						Properties: &sdk.Record{
							Name:    sdk.PtrString("a.de"),
							Type:    sdk.PtrString("A"),
							Ttl:     sdk.PtrInt32(300),
							Content: sdk.PtrString("5.6.7.8"),
							Enabled: sdk.PtrBool(true),
						},
					},
				},
			},
			expectedRecordsDeleted: nil,
		},
		{
			name: "delete the only record in a zone",
			givenZones: sdk.ZoneReadList{
				Items: &[]sdk.ZoneRead{
					{
						Id: sdk.PtrString(deZoneId),
						Properties: &sdk.Zone{
							ZoneName: sdk.PtrString("de"),
						},
					},
				},
			},
			givenZoneRecords: map[string]*sdk.RecordReadList{
				deZoneId: {
					Items: &[]sdk.RecordRead{
						{
							Id: sdk.PtrString("1"),
							Properties: &sdk.Record{
								Name:    sdk.PtrString("a.de"),
								Type:    sdk.PtrString("A"),
								Ttl:     sdk.PtrInt32(300),
								Content: sdk.PtrString("1.2.3.4"),
								Enabled: sdk.PtrBool(true),
							},
						},
					},
				},
			},
			whenChanges: &plan.Changes{
				Delete: []*endpoint.Endpoint{
					{
						DNSName:    "a.de",
						RecordType: "A",
						Targets:    []string{"1.2.3.4"},
						RecordTTL:  300,
						Labels:     map[string]string{},
					},
				},
			},
			expectedRecordsCreated: nil,
			expectedRecordsDeleted: map[string][]string{
				deZoneId: {"1"},
			},
		},
		{
			name: "delete multiple records, in different zones",
			givenZones: sdk.ZoneReadList{
				Items: &[]sdk.ZoneRead{
					{
						Id: sdk.PtrString(deZoneId),
						Properties: &sdk.Zone{
							ZoneName: sdk.PtrString("de"),
						},
					},
					{
						Id: sdk.PtrString(comZoneId),
						Properties: &sdk.Zone{
							ZoneName: sdk.PtrString("com"),
						},
					},
				},
			},
			givenZoneRecords: map[string]*sdk.RecordReadList{
				deZoneId: {
					Items: &[]sdk.RecordRead{
						{
							Id: sdk.PtrString("1"),
							Properties: &sdk.Record{
								Name:    sdk.PtrString("a.de"),
								Type:    sdk.PtrString("A"),
								Ttl:     sdk.PtrInt32(300),
								Content: sdk.PtrString("1.2.3.4"),
								Enabled: sdk.PtrBool(true),
							},
						},
						{
							Id: sdk.PtrString("2"),
							Properties: &sdk.Record{
								Name:    sdk.PtrString("a.de"),
								Type:    sdk.PtrString("A"),
								Ttl:     sdk.PtrInt32(300),
								Content: sdk.PtrString("5.6.7.8"),
								Enabled: sdk.PtrBool(true),
							},
						},
					},
				},
				comZoneId: {
					Items: &[]sdk.RecordRead{
						{
							Id: sdk.PtrString("3"),
							Properties: &sdk.Record{
								Name:    sdk.PtrString("a.com"),
								Type:    sdk.PtrString("A"),
								Ttl:     sdk.PtrInt32(300),
								Content: sdk.PtrString("11.22.33.44"),
								Enabled: sdk.PtrBool(true),
							},
						},
					},
				},
			},
			whenChanges: &plan.Changes{
				Delete: []*endpoint.Endpoint{
					{
						DNSName:    "a.de",
						RecordType: "A",
						Targets:    []string{"1.2.3.4", "5.6.7.8"},
						RecordTTL:  300,
						Labels:     map[string]string{},
					},
					{
						DNSName:    "a.com",
						RecordType: "A",
						Targets:    []string{"11.22.33.44"},
						RecordTTL:  300,
						Labels:     map[string]string{},
					},
				},
			},
			expectedRecordsCreated: nil,
			expectedRecordsDeleted: map[string][]string{
				deZoneId:  {"1", "2"},
				comZoneId: {"3"},
			},
		},
		{
			name: "delete record which is not in the zone, deletes nothing",
			givenZones: sdk.ZoneReadList{
				Items: &[]sdk.ZoneRead{
					{
						Id: sdk.PtrString(deZoneId),
						Properties: &sdk.Zone{
							ZoneName: sdk.PtrString("de"),
						},
					},
				},
			},
			givenZoneRecords: map[string]*sdk.RecordReadList{
				deZoneId: {
					Items: &[]sdk.RecordRead{},
				},
			},
			whenChanges: &plan.Changes{
				Delete: []*endpoint.Endpoint{
					{
						DNSName:    "a.de",
						RecordType: "A",
						Targets:    []string{"1.2.3.4"},
						RecordTTL:  300,
						Labels:     map[string]string{},
					},
				},
			},
			expectedRecordsCreated: nil,
			expectedRecordsDeleted: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDnsClient := &mockDNSClient{
				allRecords:  tc.givenRecords,
				allZones:    tc.givenZones,
				zoneRecords: tc.givenZoneRecords,
				returnError: tc.givenError,
			}
			provider := &Provider{client: mockDnsClient}
			err := provider.ApplyChanges(ctx, tc.whenChanges)
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

type mockDNSClient struct {
	returnError    error
	allRecords     sdk.RecordReadList
	zoneRecords    map[string]*sdk.RecordReadList
	allZones       sdk.ZoneReadList
	createdRecords map[string][]sdk.RecordCreate // zoneId -> recordCreates
	deletedRecords map[string][]string           // zoneId -> recordIds
}

func (c *mockDNSClient) GetAllRecords(ctx context.Context) (sdk.RecordReadList, error) {
	log.Debugf("GetAllRecords called")
	return c.allRecords, c.returnError
}

func (c *mockDNSClient) GetZoneRecords(ctx context.Context, zoneId string) (sdk.RecordReadList, error) {
	log.Debugf("GetZoneRecords called with zoneId %s", zoneId)
	return *c.zoneRecords[zoneId], c.returnError
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

func (c *mockDNSClient) GetZones(ctx context.Context) (sdk.ZoneReadList, error) {
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
