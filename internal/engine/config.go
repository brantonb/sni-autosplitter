// internal/engine/config.go
package engine

import (
	"time"

	"github.com/jdharms/sni-autosplitter/internal/config"
	"github.com/jdharms/sni-autosplitter/internal/sni"
	"github.com/sirupsen/logrus"
)

// EngineConfig contains configuration for the splitting engine
type EngineConfig struct {
	PollInterval time.Duration
	BufferSize   int
}

// DefaultEngineConfig returns the default engine configuration
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		PollInterval: time.Second / 60, // 60 FPS
		BufferSize:   100,
	}
}

// NewSplittingEngine creates a new splitting engine
func NewSplittingEngine(
	logger *logrus.Logger,
	client *sni.Client,
	device *sni.Device,
	runConfig *config.RunConfig,
	gameConfig *config.GameConfig,
	config *EngineConfig,
) *SplittingEngine {
	if config == nil {
		config = DefaultEngineConfig()
	}

	engine := &SplittingEngine{
		logger:            logger,
		client:            client,
		device:            device,
		evaluator:         NewConditionEvaluator(logger, client),
		session:           NewSplitterSession(runConfig, gameConfig),
		pollInterval:      config.PollInterval,
		splitSubscribers:  make(map[chan SplitEvent]struct{}),
		statusSubscribers: make(map[chan StatusEvent]struct{}),
	}

	return engine
}
