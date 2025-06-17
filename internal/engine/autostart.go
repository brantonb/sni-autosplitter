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
	maxRetries int

	// State tracking
	enabled bool
}

// AutostartConfig contains configuration for autostart detection
type AutostartConfig struct {
	MaxRetries int // Maximum consecutive failed reads before giving up
}

// DefaultAutostartConfig returns default autostart configuration
func DefaultAutostartConfig() *AutostartConfig {
	return &AutostartConfig{
		MaxRetries: 5,
	}
}

// NewAutostartDetector creates a new autostart detector
func NewAutostartDetector(logger *logrus.Logger, client *sni.Client, config *AutostartConfig) *AutostartDetector {
	if config == nil {
		config = DefaultAutostartConfig()
	}

	return &AutostartDetector{
		logger:     logger,
		client:     client,
		evaluator:  NewConditionEvaluator(logger, client),
		maxRetries: config.MaxRetries,
		enabled:    true,
	}
}

// CheckAutostart checks if autostart conditions are met
func (ad *AutostartDetector) CheckAutostart(ctx context.Context, deviceURI string, autostart *config.Split, state *SplitState) (bool, error) {
	if !ad.enabled {
		return false, nil
	}

	if autostart == nil {
		return false, fmt.Errorf("autostart configuration is nil")
	}

	// Evaluate the autostart condition
	result, err := ad.evaluateAutostartWithRetry(ctx, deviceURI, autostart, state)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate autostart condition: %w", err)
	}

	return result, nil
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
	ad.logger.Debug("Autostart detector state reset")
}

// GetStatus returns the current autostart detector status
func (ad *AutostartDetector) GetStatus() AutostartStatus {
	return AutostartStatus{
		Enabled: ad.enabled,
	}
}

// AutostartStatus contains status information about autostart detection
type AutostartStatus struct {
	Enabled bool `json:"enabled"`
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

	ad.maxRetries = config.MaxRetries

	ad.logger.WithFields(logrus.Fields{
		"max_retries": ad.maxRetries,
	}).Info("Autostart detector configuration updated")

	// Reset state to apply new configuration
	ad.Reset()
}

// GetConfiguration returns the current autostart detector configuration
func (ad *AutostartDetector) GetConfiguration() *AutostartConfig {
	return &AutostartConfig{
		MaxRetries: ad.maxRetries,
	}
}
