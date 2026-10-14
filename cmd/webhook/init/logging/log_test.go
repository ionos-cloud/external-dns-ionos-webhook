package logging

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	oldLevel, oldFormatter := log.GetLevel(), log.StandardLogger().Formatter
	t.Cleanup(func() {
		log.SetLevel(oldLevel)
		log.SetFormatter(oldFormatter)
	})

	for name, tc := range map[string]struct {
		level         string
		format        string
		expectedLevel log.Level
		expectJSON    bool
	}{
		"defaults":       {"", "", log.InfoLevel, false},
		"named level":    {"debug", "", log.DebugLevel, false},
		"numeric level":  {"5", "", log.DebugLevel, false},
		"invalid level":  {"loud", "", log.InfoLevel, false},
		"json format":    {"warn", "json", log.WarnLevel, true},
		"unknown format": {"", "xml", log.InfoLevel, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", tc.level)
			t.Setenv("LOG_FORMAT", tc.format)
			Init()
			assert.Equal(t, tc.expectedLevel, log.GetLevel())
			_, isJSON := log.StandardLogger().Formatter.(*log.JSONFormatter)
			assert.Equal(t, tc.expectJSON, isJSON)
		})
	}
}
