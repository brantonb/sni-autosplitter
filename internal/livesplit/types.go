// internal/livesplit/types.go
package livesplit

import "time"

// Command represents a basic LiveSplit One command
type Command struct {
	Command string `json:"command"`
}

// SetGameTimeCommand represents a setgametime command
type SetGameTimeCommand struct {
	Command string `json:"command"`
	Time    string `json:"time"`
}

// ResetCommand represents a reset command with optional save parameter
type ResetCommand struct {
	Command     string `json:"command"`
	SaveAttempt *bool  `json:"saveAttempt,omitempty"`
}

// SetCurrentComparisonCommand represents a comparison change command
type SetCurrentComparisonCommand struct {
	Command    string `json:"command"`
	Comparison string `json:"comparison"`
}

// SetCurrentTimingMethodCommand represents a timing method change command
type SetCurrentTimingMethodCommand struct {
	Command      string `json:"command"`
	TimingMethod string `json:"timingMethod"` // "RealTime" or "GameTime"
}

// SetCustomVariableCommand represents a custom variable command
type SetCustomVariableCommand struct {
	Command string `json:"command"`
	Key     string `json:"key"`
	Value   string `json:"value"`
}

// GetCurrentTimeCommand represents a request for current time
type GetCurrentTimeCommand struct {
	Command      string  `json:"command"`
	TimingMethod *string `json:"timingMethod,omitempty"`
}

// Event represents an event sent from LiveSplit One
type Event struct {
	Event string `json:"event"`
}

// Response represents a response from LiveSplit One
type Response struct {
	Success *interface{} `json:"success,omitempty"`
	Error   *ErrorInfo   `json:"error,omitempty"`
}

// ErrorInfo represents error information from LiveSplit One
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// TimingMethod represents the timing method used
type TimingMethod string

const (
	TimingMethodRealTime TimingMethod = "RealTime"
	TimingMethodGameTime TimingMethod = "GameTime"
)

// TimerPhase represents the current phase of the timer
type TimerPhase string

const (
	TimerPhaseNotRunning TimerPhase = "NotRunning"
	TimerPhaseRunning    TimerPhase = "Running"
	TimerPhasePaused     TimerPhase = "Paused"
	TimerPhaseEnded      TimerPhase = "Ended"
)

// GetCurrentStateResponse represents the response for GetCurrentState
type GetCurrentStateResponse struct {
	State string `json:"state"`
	Index *int   `json:"index,omitempty"`
}

// SegmentTime represents time information for a segment
type SegmentTime struct {
	RealTime *time.Duration `json:"realTime,omitempty"`
	GameTime *time.Duration `json:"gameTime,omitempty"`
}

// LiveSplitOneEvents contains all the event types that can be sent
const (
	EventSplitted            = "Splitted"
	EventUndoSplit           = "UndoSplit"
	EventSkipped             = "Skipped"
	EventStarted             = "Started"
	EventReset               = "Reset"
	EventPaused              = "Paused"
	EventResumed             = "Resumed"
	EventGameTimeInitialized = "GameTimeInitialized"
	EventGameTimePaused      = "GameTimePaused"
	EventGameTimeResumed     = "GameTimeResumed"
	EventComparisonChanged   = "ComparisonChanged"
	EventTimingMethodChanged = "TimingMethodChanged"
)

// LiveSplitOneCommands contains all the command types that can be sent
const (
	CommandStart                      = "start"
	CommandSplit                      = "split"
	CommandSplitOrStart               = "splitOrStart"
	CommandReset                      = "reset"
	CommandUndoSplit                  = "undoSplit"
	CommandSkipSplit                  = "skipSplit"
	CommandTogglePauseOrStart         = "togglePauseOrStart"
	CommandPause                      = "pause"
	CommandResume                     = "resume"
	CommandUndoAllPauses              = "undoAllPauses"
	CommandSwitchToPreviousComparison = "switchToPreviousComparison"
	CommandSwitchToNextComparison     = "switchToNextComparison"
	CommandSetCurrentComparison       = "setCurrentComparison"
	CommandToggleTimingMethod         = "toggleTimingMethod"
	CommandSetCurrentTimingMethod     = "setCurrentTimingMethod"
	CommandInitializeGameTime         = "initializeGameTime"
	CommandSetGameTime                = "setGameTime"
	CommandPauseGameTime              = "pauseGameTime"
	CommandResumeGameTime             = "resumeGameTime"
	CommandSetLoadingTimes            = "setLoadingTimes"
	CommandSetCustomVariable          = "setCustomVariable"
	CommandGetCurrentTime             = "getCurrentTime"
	CommandGetSegmentName             = "getSegmentName"
	CommandGetComparisonTime          = "getComparisonTime"
	CommandGetCurrentRunSplitTime     = "getCurrentRunSplitTime"
	CommandGetCurrentState            = "getCurrentState"
	CommandPing                       = "ping"
)

// MessageWrapper wraps messages with timestamps for logging
type MessageWrapper struct {
	Timestamp time.Time   `json:"timestamp"`
	Type      string      `json:"type"` // "command", "event", "response"
	Content   interface{} `json:"content"`
}

// ClientInfo contains information about a connected client
type ClientInfo struct {
	RemoteAddr    string    `json:"remote_addr"`
	ConnectedAt   time.Time `json:"connected_at"`
	LastMessageAt time.Time `json:"last_message_at"`
	MessageCount  int       `json:"message_count"`
}

// ServerStats contains statistics about the LiveSplit server
type ServerStats struct {
	Running          bool         `json:"running"`
	ClientCount      int          `json:"client_count"`
	Address          string       `json:"address"`
	StartTime        time.Time    `json:"start_time,omitempty"`
	TotalConnections int          `json:"total_connections"`
	MessagesSent     int          `json:"messages_sent"`
	MessagesReceived int          `json:"messages_received"`
	Clients          []ClientInfo `json:"clients"`
}
