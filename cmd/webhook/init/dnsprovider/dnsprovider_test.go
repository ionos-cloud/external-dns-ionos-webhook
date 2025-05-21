package dnsprovider

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ionoscloudsdk "github.com/ionos-cloud/sdk-go-dns"
	sdk "github.com/ionos-developer/dns-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionoscloud"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/configuration"
	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionoscore"
)

func TestInit(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	jwtPayloadEncoded := base64.RawURLEncoding.EncodeToString([]byte(`{ "something" : "we dont care" }`))
	ionoscoreAPI := mockIonosCoreAPI(t)
	defer ionoscoreAPI.Close()
	ionoscloudAPI := mockIonosCloudAPI(t)
	defer ionoscloudAPI.Close()

	cases := []struct {
		name          string
		config        configuration.Config
		env           map[string]string
		providerType  string
		expectedError string
	}{
		{
			name:   "minimal config for ionos core provider",
			config: configuration.Config{},
			env: map[string]string{
				"IONOS_API_KEY": "apikey must be there",
				"IONOS_API_URL": ionoscoreAPI.URL,
			},
			providerType: "core",
		},
		{
			name:   "config for ionos core provider, apikey with 2 dots but no jwt because no json",
			config: configuration.Config{},
			env: map[string]string{
				"IONOS_API_KEY": "algorithm.nojson.signature",
				"IONOS_API_URL": ionoscoreAPI.URL,
			},
			providerType: "core",
		},
		{
			name:   "config for ionos core provider, apikey with 2 dots but no jwt because payload not base64 encoded",
			config: configuration.Config{},
			env: map[string]string{
				"IONOS_API_KEY": "algorithm.==.signature",
				"IONOS_API_URL": ionoscoreAPI.URL,
			},
			providerType: "core",
		},
		{
			name:   "minimal config for ionos cloud provider, token can be decoded as jwt ",
			config: configuration.Config{},
			env: map[string]string{
				"IONOS_API_KEY": "algorithm." + jwtPayloadEncoded + ".signature",
				"IONOS_API_URL": ionoscloudAPI.URL,
			},
			providerType: "cloud",
		},
		{
			name:          "without api key you are not able to create provider",
			config:        configuration.Config{},
			expectedError: "reading ionos ionosConfig failed: env: environment variable \"IONOS_API_KEY\" should not be empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			dnsProvider, err := Init(tc.config)
			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError, "expecting error")
				return
			}
			assert.NoErrorf(t, err, "error creating provider")
			assert.NotNil(t, dnsProvider)
			if tc.providerType == "core" {
				_, ok := dnsProvider.(*ionoscore.Provider)
				assert.True(t, ok, "provider is not of type ionoscore.Provider")
			} else if tc.providerType == "cloud" {
				_, ok := dnsProvider.(*ionoscloud.Provider)
				assert.True(t, ok, "provider is not of type ionoscloud.Provider")
			}
		})
	}
}

func mockIonosCoreAPI(t *testing.T) *httptest.Server {
	zonesList := []sdk.Zone{
		{
			Name: sdk.PtrString("example.com"),
			Id:   sdk.PtrString("some-id"),
			Type: sdk.NATIVE.Ptr(),
		},
	}
	jsonZones, err := json.Marshal(zonesList)
	require.NoError(t, err)
	mockApi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json") // Set Content-Type header
		w.WriteHeader(http.StatusOK)
		w.Write(jsonZones) //nolint:errcheck
	}))
	return mockApi
}

func mockIonosCloudAPI(t *testing.T) *httptest.Server {
	zonesList := ionoscloudsdk.ZoneReadList{
		Id: sdk.PtrString("zone-list-id-1"),
		Items: &[]ionoscloudsdk.ZoneRead{
			{
				Id: sdk.PtrString("zone-id-1"),
				Properties: &ionoscloudsdk.Zone{
					ZoneName: sdk.PtrString("example.com"),
				},
			},
		},
	}

	jsonZones, err := json.Marshal(zonesList)
	require.NoError(t, err)
	mockApi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json") // Set Content-Type header
		w.WriteHeader(http.StatusOK)
		w.Write(jsonZones) //nolint:errcheck
	}))
	return mockApi
}
