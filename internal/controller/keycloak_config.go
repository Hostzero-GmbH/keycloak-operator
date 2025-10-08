package controller

import (
	"time"
)

// SyncPeriod defines default sync periods for controllers
const (
	// DefaultSyncPeriod is the default reconciliation period
	DefaultSyncPeriod = 10 * time.Minute

	// ShortSyncPeriod is used for resources that need faster updates
	ShortSyncPeriod = 5 * time.Minute

	// LongSyncPeriod is used for stable resources
	LongSyncPeriod = 30 * time.Minute
)

// ControllerConfig holds common controller configuration
type ControllerConfig struct {
	// SyncPeriod is the reconciliation period
	SyncPeriod time.Duration

	// MaxConcurrentReconciles is the max number of concurrent reconciles
	MaxConcurrentReconciles int
}

// DefaultControllerConfig returns default controller configuration
func DefaultControllerConfig() ControllerConfig {
	return ControllerConfig{
		SyncPeriod:              DefaultSyncPeriod,
		MaxConcurrentReconciles: 1,
	}
}
