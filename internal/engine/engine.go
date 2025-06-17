// internal/engine/engine.go
package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jdharms/sni-autosplitter/internal/config"
	"github.com/jdharms/sni-autosplitter/internal/sni"
	"github.com/sirupsen/logrus"
)

// SplittingEngine manages the autosplitting logic and state
type SplittingEngine struct {
	logger    *logrus.Logger
	client    *sni.Client
	device    *sni.Device
	evaluator *ConditionEvaluator
	session   *SplitterSession

	// Configuration
	pollInterval time.Duration

	// Runtime state
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex

	// Event channels
	splitChan  chan SplitEvent
	statusChan chan StatusEvent

	// Callbacks
	onSplit       func(splitName string, splitIndex int)
	onSkip        func(splitName string, splitIndex int)
	onUndo        func(splitName string, splitIndex int)
	onStart       func()
	onReset       func()
	onError       func(error)
	onStateChange func(oldState, newState SplitterState)
}

// SplitEvent represents a split trigger event
type SplitEvent struct {
	SplitName  string
	SplitIndex int
	Timestamp  time.Time
}

// StatusEvent represents a status update event
type StatusEvent struct {
	State     SplitterState
	Message   string
	Timestamp time.Time
}

// EngineConfig contains configuration for the splitting engine
type EngineConfig struct {
	PollInterval time.Duration
	BufferSize   int
}

// DefaultEngineConfig returns the default engine configuration
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		PollInterval: time.Millisecond * 33, // ~30 FPS
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
		logger:       logger,
		client:       client,
		device:       device,
		evaluator:    NewConditionEvaluator(logger, client),
		session:      NewSplitterSession(runConfig, gameConfig),
		pollInterval: config.PollInterval,
		splitChan:    make(chan SplitEvent, config.BufferSize),
		statusChan:   make(chan StatusEvent, config.BufferSize),
	}

	// Set up session callbacks
	engine.session.SetCallbacks(
		engine.handleStateChange,
		engine.handleSplit,
		engine.handleSkip,
		engine.handleUndo,
		engine.handleStart,
		engine.handleReset,
		engine.handlePause,
		engine.handleResume,
	)

	return engine
}

// Start begins the splitting engine
func (se *SplittingEngine) Start(ctx context.Context) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	if se.running {
		return fmt.Errorf("splitting engine is already running")
	}

	se.logger.Info("Starting splitting engine")

	// Create cancellable context
	se.ctx, se.cancel = context.WithCancel(ctx)
	se.running = true

	// Start the main loop
	go se.runLoop()

	// Initialize state
	se.session.SetState(StateWaitingForStart)
	se.logger.Info("Autostart enabled - waiting for start condition")

	return nil
}

// Stop stops the splitting engine
func (se *SplittingEngine) Stop() error {
	se.mu.Lock()
	defer se.mu.Unlock()

	if !se.running {
		return nil
	}

	se.logger.Info("Stopping splitting engine")

	se.cancel()
	se.running = false

	// Close channels
	close(se.splitChan)
	close(se.statusChan)

	return nil
}

// IsRunning returns whether the engine is running
func (se *SplittingEngine) IsRunning() bool {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.running
}

// GetSession returns the current splitter session
func (se *SplittingEngine) GetSession() *SplitterSession {
	return se.session
}

// ManualSplit manually triggers a split
func (se *SplittingEngine) ManualSplit() error {
	if !se.session.CanSplit() {
		return fmt.Errorf("cannot split in current state: %s", se.session.GetState())
	}

	se.logger.Info("Manual split triggered")
	return se.triggerSplit()
}

// ManualUndoSplit manually undoes the last split
func (se *SplittingEngine) ManualUndoSplit() error {
	se.logger.Info("Manual undo split triggered")
	return se.triggerUndoSplit()
}

// ManualSkipSplit manually skips the last split
func (se *SplittingEngine) ManualSkipSplit() error {
	se.logger.Info("Manual skip split triggered")
	return se.triggerSkipSplit()
}

// ManualReset manually resets the run
func (se *SplittingEngine) ManualReset() error {
	se.logger.Info("Manual reset triggered")
	se.session.Reset()
	se.session.SetState(StateWaitingForStart)
	se.publishStatus(StateWaitingForStart, "Run reset - waiting for autostart")
	return nil
}

// GetSplitChan returns the split event channel
func (se *SplittingEngine) GetSplitChan() <-chan SplitEvent {
	return se.splitChan
}

// GetStatusChan returns the status event channel
func (se *SplittingEngine) GetStatusChan() <-chan StatusEvent {
	return se.statusChan
}

// SetCallbacks sets the engine event callbacks
func (se *SplittingEngine) SetCallbacks(
	onSplit func(splitName string, splitIndex int),
	onStart func(),
	onReset func(),
	onError func(error),
	onStateChange func(oldState, newState SplitterState),
) {
	se.mu.Lock()
	defer se.mu.Unlock()

	se.onSplit = onSplit
	se.onStart = onStart
	se.onReset = onReset
	se.onError = onError
	se.onStateChange = onStateChange
}

// runLoop is the main engine loop that handles condition checking
func (se *SplittingEngine) runLoop() {
	se.logger.Info("Splitting engine loop started")
	ticker := time.NewTicker(se.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-se.ctx.Done():
			se.logger.Info("Splitting engine loop stopped")
			return
		case <-ticker.C:
			if err := se.tick(); err != nil {
				se.logger.WithError(err).Error("Error in splitting engine tick")
				se.handleError(err)
			}
		}
	}
}

// tick performs one iteration of condition checking
func (se *SplittingEngine) tick() error {
	state := se.session.GetState()

	switch state {
	case StateWaitingForStart:
		return se.checkAutostart()
	case StateRunning:
		return se.checkCurrentSplit()
	case StatePaused, StateCompleted, StateError:
		// No action needed in these states
		return nil
	default:
		return fmt.Errorf("unknown splitter state: %s", state)
	}
}

// checkAutostart checks if the autostart condition is met
func (se *SplittingEngine) checkAutostart() error {
	autostart := se.session.GetGameConfig().Autostart
	autostartState := se.session.GetAutostartState()

	result, err := se.evaluator.EvaluateComplexCondition(se.ctx, se.device.URI, &autostart, autostartState)
	if err != nil {
		return fmt.Errorf("failed to evaluate autostart condition: %w", err)
	}

	if result {
		se.logger.Info("Autostart condition met - starting run")
		se.session.Start()
		se.publishStatus(StateRunning, "Run started")
	}

	return nil
}

// checkCurrentSplit checks if the current split condition is met
func (se *SplittingEngine) checkCurrentSplit() error {
	currentSplitName := se.session.GetCurrentSplitName()
	if currentSplitName == "" {
		// No more splits - run should be completed
		if se.session.GetState() == StateRunning {
			se.session.SetState(StateCompleted)
		}
		return nil
	}

	// Get the split configuration
	split, err := se.session.GetGameConfig().GetSplitByName(currentSplitName)
	if err != nil {
		return fmt.Errorf("failed to get split configuration for '%s': %w", currentSplitName, err)
	}

	// Get the split state for sequential conditions
	splitState := se.session.GetSplitState(currentSplitName)

	// Evaluate the split condition
	result, err := se.evaluator.EvaluateComplexCondition(se.ctx, se.device.URI, split, splitState)
	if err != nil {
		return fmt.Errorf("failed to evaluate split condition for '%s': %w", currentSplitName, err)
	}

	if result {
		se.logger.WithField("split", currentSplitName).Info("Split condition met")
		return se.triggerSplit()
	}

	return nil
}

// triggerSplit triggers a split and handles the transition
func (se *SplittingEngine) triggerSplit() error {
	if !se.session.TriggerSplit() {
		return fmt.Errorf("failed to trigger split")
	}

	// Check if run is completed
	if se.session.GetState() == StateCompleted {
		se.logger.Info("Run completed!")
		se.publishStatus(StateCompleted, "Run completed")
	}

	return nil
}

// triggerUndoSplit triggers an undo split and handles the transition
func (se *SplittingEngine) triggerUndoSplit() error {
	if !se.session.TriggerUndoSplit() {
		return fmt.Errorf("failed to trigger undo split")
	}

	return nil
}

// triggerSkipSplit triggers a skip split and handles the transition
func (se *SplittingEngine) triggerSkipSplit() error {
	if !se.session.TriggerSkipSplit() {
		return fmt.Errorf("failed to trigger skip split")
	}

	return nil
}

// handleStateChange handles state change events from the session
func (se *SplittingEngine) handleStateChange(oldState, newState SplitterState) {
	se.logger.WithFields(logrus.Fields{
		"old_state": oldState.String(),
		"new_state": newState.String(),
	}).Info("Splitter state changed")

	se.publishStatus(newState, fmt.Sprintf("State changed from %s to %s", oldState.String(), newState.String()))

	if se.onStateChange != nil {
		go se.onStateChange(oldState, newState)
	}
}

// handleSplit handles split events from the session
func (se *SplittingEngine) handleSplit(splitName string, splitIndex int) {
	se.logger.WithFields(logrus.Fields{
		"split_name":  splitName,
		"split_index": splitIndex,
		"total":       se.session.GetTotalSplits(),
	}).Info("Split triggered")

	// Publish split event
	select {
	case se.splitChan <- SplitEvent{
		SplitName:  splitName,
		SplitIndex: splitIndex,
		Timestamp:  time.Now(),
	}:
	default:
		se.logger.Warn("Split event channel is full")
	}

	if se.onSplit != nil {
		go se.onSplit(splitName, splitIndex)
	}
}

// handleStart handles start events from the session
func (se *SplittingEngine) handleStart() {
	se.logger.Info("Run started")
	se.publishStatus(StateRunning, "Run started")

	if se.onStart != nil {
		go se.onStart()
	}
}

// handleReset handles reset events from the session
func (se *SplittingEngine) handleReset() {
	se.logger.Info("Run reset")

	se.publishStatus(StateWaitingForStart, "Run reset - waiting for autostart")

	if se.onReset != nil {
		go se.onReset()
	}
}

// handlePause handles pause events from the session
func (se *SplittingEngine) handlePause() {
	se.logger.Info("Run paused")
	se.publishStatus(StatePaused, "Run paused")
}

// handleResume handles resume events from the session
func (se *SplittingEngine) handleResume() {
	se.logger.Info("Run resumed")
	se.publishStatus(StateRunning, "Run resumed")
}

// handleError handles errors in the engine
func (se *SplittingEngine) handleError(err error) {
	se.logger.WithError(err).Error("Splitting engine error")
	se.session.SetError()
	se.publishStatus(StateError, fmt.Sprintf("Error: %v", err))

	if se.onError != nil {
		go se.onError(err)
	}
}

// handleSkip handles skip events from the session
func (se *SplittingEngine) handleSkip(splitName string, splitIndex int) {
	se.logger.WithFields(logrus.Fields{
		"split_name":  splitName,
		"split_index": splitIndex,
	}).Info("Split skipped")

	if se.onSkip != nil {
		go se.onSkip(splitName, splitIndex)
	}
}

// handleUndo handles undo events from the session
func (se *SplittingEngine) handleUndo(splitName string, splitIndex int) {
	se.logger.WithFields(logrus.Fields{
		"split_name":  splitName,
		"split_index": splitIndex,
	}).Info("Split undone")

	if se.onUndo != nil {
		go se.onUndo(splitName, splitIndex)
	}
}

// publishStatus publishes a status event
func (se *SplittingEngine) publishStatus(state SplitterState, message string) {
	select {
	case se.statusChan <- StatusEvent{
		State:     state,
		Message:   message,
		Timestamp: time.Now(),
	}:
	default:
		se.logger.Warn("Status event channel is full")
	}
}

// GetStats returns current engine statistics
func (se *SplittingEngine) GetStats() EngineStats {
	session := se.session

	return EngineStats{
		State:              session.GetState(),
		CurrentSplit:       session.GetCurrentSplit(),
		CurrentSplitName:   session.GetCurrentSplitName(),
		TotalSplits:        session.GetTotalSplits(),
		Progress:           session.GetProgress(),
		ElapsedTime:        session.GetElapsedTime(),
		TimeSinceLastSplit: session.GetTimeSinceLastSplit(),
		RunName:            session.GetRunConfig().Name,
		GameName:           session.GetGameConfig().GetGameName(),
		IsRunning:          se.IsRunning(),
	}
}

// EngineStats contains statistics about the engine state
type EngineStats struct {
	State              SplitterState `json:"state"`
	CurrentSplit       int           `json:"current_split"`
	CurrentSplitName   string        `json:"current_split_name"`
	TotalSplits        int           `json:"total_splits"`
	Progress           float64       `json:"progress"`
	ElapsedTime        time.Duration `json:"elapsed_time"`
	TimeSinceLastSplit time.Duration `json:"time_since_last_split"`
	RunName            string        `json:"run_name"`
	GameName           string        `json:"game_name"`
	IsRunning          bool          `json:"is_running"`
}

// PauseEngine pauses the splitting engine
func (se *SplittingEngine) PauseEngine() error {
	if se.session.GetState() != StateRunning {
		return fmt.Errorf("cannot pause in current state: %s", se.session.GetState())
	}

	se.session.Pause()
	return nil
}

// ResumeEngine resumes the splitting engine
func (se *SplittingEngine) ResumeEngine() error {
	if se.session.GetState() != StatePaused {
		return fmt.Errorf("cannot resume in current state: %s", se.session.GetState())
	}

	se.session.Resume()
	return nil
}

// GetCurrentSplitCondition returns the current split's condition details for debugging
func (se *SplittingEngine) GetCurrentSplitCondition() (*config.Split, *SplitState, error) {
	currentSplitName := se.session.GetCurrentSplitName()
	if currentSplitName == "" {
		return nil, nil, fmt.Errorf("no current split")
	}

	split, err := se.session.GetGameConfig().GetSplitByName(currentSplitName)
	if err != nil {
		return nil, nil, err
	}

	splitState := se.session.GetSplitState(currentSplitName)
	return split, splitState, nil
}

// TestCondition tests a specific condition without affecting the run state
func (se *SplittingEngine) TestCondition(split *config.Split) (bool, error) {
	// Create a temporary split state for testing
	tempState := NewSplitState()

	return se.evaluator.EvaluateComplexCondition(se.ctx, se.device.URI, split, tempState)
}
