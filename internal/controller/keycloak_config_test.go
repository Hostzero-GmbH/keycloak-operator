package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultControllerConfig(t *testing.T) {
	cfg := DefaultControllerConfig()

	assert.Equal(t, DefaultSyncPeriod, cfg.SyncPeriod)
	assert.Equal(t, 1, cfg.MaxConcurrentReconciles)
}

func TestSyncPeriodConstants(t *testing.T) {
	assert.Equal(t, 10*time.Minute, DefaultSyncPeriod)
	assert.Equal(t, 5*time.Minute, ShortSyncPeriod)
	assert.Equal(t, 30*time.Minute, LongSyncPeriod)

	// Short should be less than default
	assert.Less(t, ShortSyncPeriod, DefaultSyncPeriod)
	// Default should be less than long
	assert.Less(t, DefaultSyncPeriod, LongSyncPeriod)
}
