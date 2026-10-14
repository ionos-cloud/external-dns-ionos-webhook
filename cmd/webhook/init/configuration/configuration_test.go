package configuration

import (
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestInitDefaults(t *testing.T) {
	cfg := Init()
	assert.Equal(t, "localhost", cfg.ServerHost)
	assert.Equal(t, 8888, cfg.ServerPort)
	assert.Equal(t, "0.0.0.0", cfg.MetricsHost)
	assert.Equal(t, 8080, cfg.MetricsPort)
	assert.Empty(t, cfg.DomainFilter)
	assert.Zero(t, cfg.ServerReadTimeout)
}

func TestInitFromEnvironment(t *testing.T) {
	t.Setenv("SERVER_HOST", "0.0.0.0")
	t.Setenv("SERVER_PORT", "9999")
	t.Setenv("METRICS_PORT", "9090")
	t.Setenv("SERVER_READ_TIMEOUT", "5s")
	t.Setenv("DOMAIN_FILTER", "a.de,b.de")
	t.Setenv("EXCLUDE_DOMAIN_FILTER", "c.de")
	t.Setenv("REGEXP_DOMAIN_FILTER", ".*")
	cfg := Init()
	assert.Equal(t, "0.0.0.0", cfg.ServerHost)
	assert.Equal(t, 9999, cfg.ServerPort)
	assert.Equal(t, 9090, cfg.MetricsPort)
	assert.Equal(t, 5*time.Second, cfg.ServerReadTimeout)
	assert.Equal(t, []string{"a.de", "b.de"}, cfg.DomainFilter)
	assert.Equal(t, []string{"c.de"}, cfg.ExcludeDomains)
	assert.Equal(t, ".*", cfg.RegexDomainFilter)
}

func TestInitInvalidEnvironmentIsFatal(t *testing.T) {
	logger := log.StandardLogger()
	oldExit := logger.ExitFunc
	t.Cleanup(func() { logger.ExitFunc = oldExit })
	exitCode := -1
	logger.ExitFunc = func(code int) { exitCode = code }

	t.Setenv("SERVER_PORT", "not-a-number")
	Init()
	assert.Equal(t, 1, exitCode)
}
