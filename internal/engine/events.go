// internal/engine/events.go
package engine

import (
	"context"
	"time"
)

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
