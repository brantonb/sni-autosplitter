// internal/engine/autostart.go
package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/jdharms/sni-autosplitter/internal/config"
	"github.com/jdharms/sni-autosplitter/internal/sni"
	"github.com/sirupsen/logrus"
)

// AutostartDetector handles detection of autostart conditions
type AutostartDetector struct {
	logger    *logrus.Logger
	client    *sni.Client
	evaluator *ConditionEvaluator

	// Configuration
	debounceTime  time.Duration
	stabilityTime time.Duration
	maxRetries    int

	// State tracking
	lastResult       bool
	lastChangeTime   time.Time
	consecutiveCount int
	enabled          bool
}

// AutostartConfig contains configuration for autostart detection
type AutostartConfig struct {
	DebounceTime  time.Duration // Time to wait for condition stability
	StabilityTime time.Duration // Time condition must remain stable
	MaxRetries    int           // Maximum consecutive failed reads before giving up
}

// DefaultAutostartConfig returns default autostart configuration
func DefaultAutostartConfig() *AutostartConfig {
	return &AutostartConfig{
		DebounceTime:  time.Millisecond * 100,
		StabilityTime: time.Millisecond * 300,
		MaxRetries:    5,
	}
}

// NewAutostartDetector creates a new autostart detector
func NewAutostartDetector(logger *logrus.Logger, client *sni.Client, config *AutostartConfig) *AutostartDetector {
	if config == nil {
		config = DefaultAutostartConfig()
	}

	return &AutostartDetector{
		logger:        logger,
		client:        client,
		evaluator:     NewConditionEvaluator(logger, client),
		debounceTime:  config.DebounceTime,
		stabilityTime: config.StabilityTime,
		maxRetries:    config.MaxRetries,
		enabled:       true,
	}
}

// CheckAutostart checks if autostart conditions are met with debouncing
func (ad *AutostartDetector) CheckAutostart(ctx context.Context, deviceURI string, autostart *config.Split, state *SplitState) (bool, error) {
	if !ad.enabled {
		return false, nil
	}

	if autostart == nil {
		return false, fmt.Errorf("autostart configuration is nil")
	}

	// Check if autostart is active
	if autostart.Active != "1" {
		ad.logger.Debug("Autostart is disabled in configuration")
		return false, nil
	}

	// Evaluate the autostart condition
	result, err := ad.evaluateAutostartWithRetry(ctx, deviceURI, autostart, state)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate autostart condition: %w", err)
	}

	// Apply debouncing logic
	return ad.applyDebouncing(result), nil
}

// evaluateAutostartWithRetry evaluates autostart condition with retry logic
func (ad *AutostartDetector) evaluateAutostartWithRetry(ctx context.Context, deviceURI string, autostart *config.Split, state *SplitState) (bool, error) {
	var lastErr error

	for attempt := 0; attempt < ad.maxRetries; attempt++ {
		result, err := ad.evaluator.EvaluateComplexCondition(ctx, deviceURI, autostart, state)
		if err == nil {
			if attempt > 0 {
				ad.logger.WithField("attempts", attempt+1).Debug("Autostart evaluation succeeded after retry")
			}
			return result, nil
		}

		lastErr = err
		ad.logger.WithError(err).WithField("attempt", attempt+1).Warn("Autostart evaluation failed, retrying")

		// Short delay before retry
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(time.Millisecond * 50):
		}
	}

	return false, fmt.Errorf("autostart evaluation failed after %d attempts: %w", ad.maxRetries, lastErr)
}

// applyDebouncing applies debouncing logic to prevent false triggers
func (ad *AutostartDetector) applyDebouncing(currentResult bool) bool {
	now := time.Now()

	// Check if result has changed
	if currentResult != ad.lastResult {
		ad.logger.WithFields(logrus.Fields{
			"previous": ad.lastResult,
			"current":  currentResult,
		}).Debug("Autostart condition result changed")

		ad.lastResult = currentResult
		ad.lastChangeTime = now
		ad.consecutiveCount = 1
		return false // Don't trigger immediately on change
	}

	// Result is stable - increment consecutive count
	ad.consecutiveCount++

	// If condition is false, no need to debounce
	if !currentResult {
		return false
	}

	// Check if condition has been stable long enough
	timeSinceChange := now.Sub(ad.lastChangeTime)
	if timeSinceChange >= ad.stabilityTime {
		ad.logger.WithFields(logrus.Fields{
			"stability_time": timeSinceChange,
			"consecutive":    ad.consecutiveCount,
		}).Info("Autostart condition is stable and met")
		return true
	}

	ad.logger.WithFields(logrus.Fields{
		"time_remaining": ad.stabilityTime - timeSinceChange,
		"consecutive":    ad.consecutiveCount,
	}).Debug("Autostart condition met but waiting for stability")

	return false
}

// Enable enables autostart detection
func (ad *AutostartDetector) Enable() {
	ad.enabled = true
	ad.logger.Info("Autostart detection enabled")
}

// Disable disables autostart detection
func (ad *AutostartDetector) Disable() {
	ad.enabled = false
	ad.logger.Info("Autostart detection disabled")
}

// IsEnabled returns whether autostart detection is enabled
func (ad *AutostartDetector) IsEnabled() bool {
	return ad.enabled
}

// Reset resets the autostart detector state
func (ad *AutostartDetector) Reset() {
	ad.lastResult = false
	ad.lastChangeTime = time.Time{}
	ad.consecutiveCount = 0
	ad.logger.Debug("Autostart detector state reset")
}

// GetStatus returns the current autostart detector status
func (ad *AutostartDetector) GetStatus() AutostartStatus {
	timeSinceChange := time.Duration(0)
	if !ad.lastChangeTime.IsZero() {
		timeSinceChange = time.Since(ad.lastChangeTime)
	}

	return AutostartStatus{
		Enabled:          ad.enabled,
		LastResult:       ad.lastResult,
		TimeSinceChange:  timeSinceChange,
		ConsecutiveCount: ad.consecutiveCount,
		IsStable:         timeSinceChange >= ad.stabilityTime,
		StabilityTime:    ad.stabilityTime,
	}
}

// AutostartStatus contains status information about autostart detection
type AutostartStatus struct {
	Enabled          bool          `json:"enabled"`
	LastResult       bool          `json:"last_result"`
	TimeSinceChange  time.Duration `json:"time_since_change"`
	ConsecutiveCount int           `json:"consecutive_count"`
	IsStable         bool          `json:"is_stable"`
	StabilityTime    time.Duration `json:"stability_time"`
}

// ValidateAutostartCondition validates that an autostart condition is properly configured
func (ad *AutostartDetector) ValidateAutostartCondition(autostart *config.Split) error {
	if autostart == nil {
		return fmt.Errorf("autostart configuration is nil")
	}

	if autostart.Address == "" {
		return fmt.Errorf("autostart address is required")
	}

	if autostart.Value == "" {
		return fmt.Errorf("autostart value is required")
	}

	if autostart.Type == "" {
		return fmt.Errorf("autostart type is required")
	}

	// Validate that the condition type is supported
	if !config.ValidTypes[autostart.Type] {
		return fmt.Errorf("unsupported autostart condition type: %s", autostart.Type)
	}

	// Warn about complex autostart conditions
	if len(autostart.More) > 0 {
		ad.logger.Warn("Autostart has 'more' conditions - this may make autostart detection less reliable")
	}

	if len(autostart.Next) > 0 {
		ad.logger.Warn("Autostart has 'next' conditions - this may make autostart detection less reliable")
	}

	return nil
}

// TestAutostartCondition tests an autostart condition once without state tracking
func (ad *AutostartDetector) TestAutostartCondition(ctx context.Context, deviceURI string, autostart *config.Split) (bool, error) {
	if err := ad.ValidateAutostartCondition(autostart); err != nil {
		return false, err
	}

	// Create a temporary state for testing
	tempState := NewSplitState()

	return ad.evaluator.EvaluateComplexCondition(ctx, deviceURI, autostart, tempState)
}

// SetConfiguration updates the autostart detector configuration
func (ad *AutostartDetector) SetConfiguration(config *AutostartConfig) {
	if config == nil {
		config = DefaultAutostartConfig()
	}

	ad.debounceTime = config.DebounceTime
	ad.stabilityTime = config.StabilityTime
	ad.maxRetries = config.MaxRetries

	ad.logger.WithFields(logrus.Fields{
		"debounce_time":  ad.debounceTime,
		"stability_time": ad.stabilityTime,
		"max_retries":    ad.maxRetries,
	}).Info("Autostart detector configuration updated")

	// Reset state to apply new configuration
	ad.Reset()
}

// GetConfiguration returns the current autostart detector configuration
func (ad *AutostartDetector) GetConfiguration() *AutostartConfig {
	return &AutostartConfig{
		DebounceTime:  ad.debounceTime,
		StabilityTime: ad.stabilityTime,
		MaxRetries:    ad.maxRetries,
	}
}

// MonitorAutostart continuously monitors autostart conditions (for standalone use)
func (ad *AutostartDetector) MonitorAutostart(ctx context.Context, deviceURI string, autostart *config.Split, pollInterval time.Duration) (<-chan bool, <-chan error) {
	triggerChan := make(chan bool, 1)
	errorChan := make(chan error, 1)

	go func() {
		defer close(triggerChan)
		defer close(errorChan)

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		state := NewSplitState()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				triggered, err := ad.CheckAutostart(ctx, deviceURI, autostart, state)
				if err != nil {
					select {
					case errorChan <- err:
					case <-ctx.Done():
						return
					}
					continue
				}

				if triggered {
					select {
					case triggerChan <- true:
					case <-ctx.Done():
						return
					}
					// Reset after successful trigger
					ad.Reset()
					state.Reset()
				}
			}
		}
	}()

	return triggerChan, errorChan
}
