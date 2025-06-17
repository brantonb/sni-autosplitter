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

	// Event subscribers
	splitSubscribers  map[chan SplitEvent]struct{}
	statusSubscribers map[chan StatusEvent]struct{}
	subscriberMu      sync.RWMutex
}

// SplitAction represents the type of split action
type SplitAction int

const (
	SplitActionSplit SplitAction = iota
	SplitActionSkip
	SplitActionUndo
)

// String returns the string representation of the split action
func (sa SplitAction) String() string {
	switch sa {
	case SplitActionSplit:
		return "Split"
	case SplitActionSkip:
		return "Skip"
	case SplitActionUndo:
		return "Undo"
	default:
		return "Unknown"
	}
}

// SplitEvent represents a split trigger event
type SplitEvent struct {
	Action     SplitAction
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

	// Close all subscriber channels
	se.subscriberMu.Lock()
	for ch := range se.splitSubscribers {
		close(ch)
	}
	for ch := range se.statusSubscribers {
		close(ch)
	}
	se.splitSubscribers = make(map[chan SplitEvent]struct{})
	se.statusSubscribers = make(map[chan StatusEvent]struct{})
	se.subscriberMu.Unlock()

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

	// Publish reset status
	se.logger.Info("Run reset")
	se.publishStatus(StateWaitingForStart, "Run reset - waiting for autostart")

	return nil
}

// RegisterSplitChannel registers a new split event subscriber
func (se *SplittingEngine) RegisterSplitChannel(ctx context.Context) <-chan SplitEvent {
	se.subscriberMu.Lock()
	defer se.subscriberMu.Unlock()

	ch := make(chan SplitEvent, 100) // Buffer size of 100 events
	se.splitSubscribers[ch] = struct{}{}

	// Start goroutine to handle context cancellation
	go func() {
		<-ctx.Done()
		se.subscriberMu.Lock()
		delete(se.splitSubscribers, ch)
		close(ch)
		se.subscriberMu.Unlock()
	}()

	return ch
}

// RegisterStatusChannel registers a new status event subscriber
func (se *SplittingEngine) RegisterStatusChannel(ctx context.Context) <-chan StatusEvent {
	se.subscriberMu.Lock()
	defer se.subscriberMu.Unlock()

	ch := make(chan StatusEvent, 100) // Buffer size of 100 events
	se.statusSubscribers[ch] = struct{}{}

	// Start goroutine to handle context cancellation
	go func() {
		<-ctx.Done()
		se.subscriberMu.Lock()
		delete(se.statusSubscribers, ch)
		close(ch)
		se.subscriberMu.Unlock()
	}()

	return ch
}

// handleError handles errors in the engine
func (se *SplittingEngine) handleError(err error) {
	se.logger.WithError(err).Error("Splitting engine error")
	se.session.SetError()
	se.publishStatus(StateError, fmt.Sprintf("Error: %v", err))
}

// publishStatus publishes a status event
func (se *SplittingEngine) publishStatus(state SplitterState, message string) {
	// Create status event
	event := StatusEvent{
		State:     state,
		Message:   message,
		Timestamp: time.Now(),
	}

	// Publish the event to all subscribers
	se.publishStatusEvent(event)
}

// publishStatusEvent publishes a status event to all subscribers
func (se *SplittingEngine) publishStatusEvent(event StatusEvent) {
	// Fan out to all subscribers
	se.subscriberMu.RLock()
	for ch := range se.statusSubscribers {
		select {
		case ch <- event:
		default:
			se.logger.Warn("Status event subscriber channel is full")
		}
	}
	se.subscriberMu.RUnlock()
}

// publishSplitEvent publishes a split event to all subscribers
func (se *SplittingEngine) publishSplitEvent(event SplitEvent) {
	// Fan out to all subscribers
	se.subscriberMu.RLock()
	for ch := range se.splitSubscribers {
		select {
		case ch <- event:
		default:
			se.logger.Warn("Split event subscriber channel is full")
		}
	}
	se.subscriberMu.RUnlock()
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

	// Handle pause event
	se.logger.Info("Run paused")
	se.publishStatus(StatePaused, "Run paused")

	return nil
}

// ResumeEngine resumes the splitting engine
func (se *SplittingEngine) ResumeEngine() error {
	if se.session.GetState() != StatePaused {
		return fmt.Errorf("cannot resume in current state: %s", se.session.GetState())
	}

	se.session.Resume()

	// Handle resume event
	se.logger.Info("Run resumed")
	se.publishStatus(StateRunning, "Run resumed")

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

		// Handle the start event
		se.logger.Info("Run started")
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
	// Get split info before triggering
	splitName := se.session.GetCurrentSplitName()
	splitIndex := se.session.GetCurrentSplit()

	if !se.session.TriggerSplit() {
		return fmt.Errorf("failed to trigger split")
	}

	// Log and publish the split event
	se.logger.WithFields(logrus.Fields{
		"split_name":  splitName,
		"split_index": splitIndex,
		"total":       se.session.GetTotalSplits(),
	}).Info("Split triggered")

	event := SplitEvent{
		Action:     SplitActionSplit,
		SplitName:  splitName,
		SplitIndex: splitIndex,
		Timestamp:  time.Now(),
	}
	se.publishSplitEvent(event)

	// Check if run is completed
	if se.session.GetState() == StateCompleted {
		se.logger.Info("Run completed!")
		se.publishStatus(StateCompleted, "Run completed")
	}

	return nil
}

// triggerUndoSplit triggers an undo split and handles the transition
func (se *SplittingEngine) triggerUndoSplit() error {
	// Get the split we're about to undo (which is the previous split)
	splitIndex := se.session.GetCurrentSplit() - 1
	if splitIndex < 0 {
		return fmt.Errorf("no split to undo")
	}
	splitName := se.session.GetRunConfig().Splits[splitIndex]

	if !se.session.TriggerUndoSplit() {
		return fmt.Errorf("failed to trigger undo split")
	}

	// Log and publish the undo event
	se.logger.WithFields(logrus.Fields{
		"split_name":  splitName,
		"split_index": splitIndex,
	}).Info("Split undone")

	event := SplitEvent{
		Action:     SplitActionUndo,
		SplitName:  splitName,
		SplitIndex: splitIndex,
		Timestamp:  time.Now(),
	}
	se.publishSplitEvent(event)

	return nil
}

// triggerSkipSplit triggers a skip split and handles the transition
func (se *SplittingEngine) triggerSkipSplit() error {
	// Get split info before triggering
	splitName := se.session.GetCurrentSplitName()
	splitIndex := se.session.GetCurrentSplit()

	if !se.session.TriggerSkipSplit() {
		return fmt.Errorf("failed to trigger skip split")
	}

	// Log and publish the skip event
	se.logger.WithFields(logrus.Fields{
		"split_name":  splitName,
		"split_index": splitIndex,
	}).Info("Split skipped")

	event := SplitEvent{
		Action:     SplitActionSkip,
		SplitName:  splitName,
		SplitIndex: splitIndex,
		Timestamp:  time.Now(),
	}
	se.publishSplitEvent(event)

	return nil
}
