package dnsprovider

import (
	"encoding/base64"
	"testing"

	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionoscloud"

	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/configuration"
	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionoscore"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	jwtPayloadEncoded := base64.RawURLEncoding.EncodeToString([]byte(`{ "something" : "we dont care" }`))

	cases := []struct {
		name          string
		config        configuration.Config
		env           map[string]string
		providerType  string
		expectedError string
	}{
		{
			name:         "minimal config for ionos core provider",
			config:       configuration.Config{},
			env:          map[string]string{"IONOS_API_KEY": "apikey must be there"},
			providerType: "core",
		},
		{
			name:         "config for ionos core provider, apikey with 2 dots but no jwt because no json",
			config:       configuration.Config{},
			env:          map[string]string{"IONOS_API_KEY": "algorithm.nojson.signature"},
			providerType: "core",
		},
		{
			name:         "config for ionos core provider, apikey with 2 dots but no jwt because payload not base64 encoded",
			config:       configuration.Config{},
			env:          map[string]string{"IONOS_API_KEY": "algorithm.==.signature"},
			providerType: "core",
		},
		{
			name:   "minimal config for ionos cloud provider, token can be decoded as jwt ",
			config: configuration.Config{},
			env: map[string]string{
				"IONOS_API_KEY": "algorithm." + jwtPayloadEncoded + ".signature",
			},
			providerType: "cloud",
		},
		{
			name:          "without api key and no username/password you are not able to create provider",
			config:        configuration.Config{},
			expectedError: "missing credentials: either IONOS_API_KEY or IONOS_USERNAME and IONOS_PASSWORD should be provided",
		},
		{
			name:   "without api key and empty password you are not able to create provider",
			config: configuration.Config{},
			env: map[string]string{
				"IONOS_USERNAME": "username",
				"IONOS_PASSWORD": "",
			},
			expectedError: "missing credentials: either IONOS_API_KEY or IONOS_USERNAME and IONOS_PASSWORD should be provided",
		},
		{
			name:   "without api key and empty username you are not able to create provider",
			config: configuration.Config{},
			env: map[string]string{
				"IONOS_USERNAME": "",
				"IONOS_PASSWORD": "password",
			},
			expectedError: "missing credentials: either IONOS_API_KEY or IONOS_USERNAME and IONOS_PASSWORD should be provided",
		},
		{
			name:   "with username and password and invalid TTL (below lower bound), returns error ",
			config: configuration.Config{},
			env: map[string]string{
				"IONOS_USERNAME":  "username",
				"IONOS_PASSWORD":  "password",
				"IONOS_TOKEN_TTL": "20s",
			},
			expectedError: "a token TTL must be between 1 hour and 1 year",
		},
		{
			name:   "with username and password and invalid TTL (above upper bound), returns error",
			config: configuration.Config{},
			env: map[string]string{
				"IONOS_USERNAME":  "username",
				"IONOS_PASSWORD":  "password",
				"IONOS_TOKEN_TTL": "41536000s",
			},
			expectedError: "a token TTL must be between 1 hour and 1 year",
		},
		{
			name:   "with username and password and valid TTL, creates cloud provider",
			config: configuration.Config{},
			env: map[string]string{
				"IONOS_USERNAME":  "username",
				"IONOS_PASSWORD":  "password",
				"IONOS_TOKEN_TTL": "720h",
			},
			providerType: "cloud",
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
