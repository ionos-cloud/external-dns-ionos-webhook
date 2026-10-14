package dnsprovider

import (
	"testing"

	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/configuration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitDomainFilters(t *testing.T) {
	for name, cfg := range map[string]configuration.Config{
		"regex filter":                {RegexDomainFilter: `.*\.a\.de`},
		"regex filter with exclusion": {RegexDomainFilter: `.*\.a\.de`, RegexDomainExclusion: `x\.a\.de`},
		"domain filter":               {DomainFilter: []string{"a.de"}},
		"exclude domains":             {ExcludeDomains: []string{"b.de"}},
		"domain filter and exclusion": {DomainFilter: []string{"a.de"}, ExcludeDomains: []string{"b.de"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("IONOS_API_KEY", "key")
			p, err := Init(cfg)
			require.NoError(t, err)
			assert.NotNil(t, p)
		})
	}
}

func TestInitInvalidEnvironment(t *testing.T) {
	t.Setenv("IONOS_API_KEY", "key")
	t.Setenv("IONOS_DEBUG", "not-a-bool")
	p, err := Init(configuration.Config{})
	assert.Nil(t, p)
	assert.ErrorContains(t, err, "reading ionos ionosConfig failed")
}
