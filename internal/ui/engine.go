// internal/ui/engine.go
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jdharms/sni-autosplitter/internal/config"
	"github.com/jdharms/sni-autosplitter/internal/engine"
	"github.com/jdharms/sni-autosplitter/internal/sni"
	"github.com/sirupsen/logrus"
)

// EngineController manages the splitting engine lifecycle and provides UI feedback
type EngineController struct {
	logger *logrus.Logger
	cli    *CLI
	engine *engine.SplittingEngine
}

// NewEngineController creates a new engine controller
func NewEngineController(logger *logrus.Logger, cli *CLI) *EngineController {
	return &EngineController{
		logger: logger,
		cli:    cli,
	}
}

// InitializeEngine creates and configures the splitting engine
func (ec *EngineController) InitializeEngine(
	sniClient *sni.Client,
	device *sni.Device,
	runConfig *config.RunConfig,
	gameConfig *config.GameConfig,
) error {
	ec.cli.printInfo("Initializing splitting engine...")

	// Create engine configuration
	engineConfig := engine.DefaultEngineConfig()

	// Create the splitting engine
	ec.engine = engine.NewSplittingEngine(
		ec.logger,
		sniClient,
		device,
		runConfig,
		gameConfig,
		engineConfig,
	)

	// Set up engine callbacks
	ec.engine.SetCallbacks(
		ec.onSplit,
		ec.onStart,
		ec.onReset,
		ec.onError,
		ec.onStateChange,
	)

	ec.cli.printSuccess("Splitting engine initialized")
	return nil
}

// StartEngine starts the splitting engine
func (ec *EngineController) StartEngine(ctx context.Context) error {
	if ec.engine == nil {
		return fmt.Errorf("engine not initialized")
	}

	ec.cli.printInfo("Starting splitting engine...")

	if err := ec.engine.Start(ctx); err != nil {
		return fmt.Errorf("failed to start engine: %w", err)
	}

	// Start monitoring engine events
	go ec.monitorEngineEvents(ctx)

	ec.cli.printSuccess("Splitting engine started")
	ec.displayEngineStatus()

	return nil
}

// StopEngine stops the splitting engine
func (ec *EngineController) StopEngine() error {
	if ec.engine == nil {
		return nil
	}

	ec.cli.printInfo("Stopping splitting engine...")

	if err := ec.engine.Stop(); err != nil {
		return fmt.Errorf("failed to stop engine: %w", err)
	}

	ec.cli.printSuccess("Splitting engine stopped")
	return nil
}

// RunInteractiveMode runs the interactive engine control mode
func (ec *EngineController) RunInteractiveMode(ctx context.Context) error {
	if ec.engine == nil {
		return fmt.Errorf("engine not initialized")
	}

	ec.cli.printInfo("Entering interactive mode")
	ec.displayCommands()

	for {
		fmt.Print("\nCommand (h for help): ")

		if !ec.cli.scanner.Scan() {
			break
		}

		command := ec.cli.scanner.Text()
		if err := ec.handleCommand(ctx, command); err != nil {
			if err.Error() == "quit" {
				break
			}
			ec.cli.printError(fmt.Sprintf("Command error: %v", err))
		}
	}

	return nil
}

// handleCommand processes interactive commands
func (ec *EngineController) handleCommand(ctx context.Context, command string) error {
	switch command {
	case "h", "help":
		ec.displayCommands()
	case "s", "status":
		ec.displayEngineStatus()
	case "start":
		return ec.handleManualStart()
	case "split":
		return ec.handleManualSplit()
	case "reset":
		return ec.handleManualReset()
	case "pause":
		return ec.handlePause()
	case "resume":
		return ec.handleResume()
	case "test":
		return ec.handleTestCondition()
	case "stats":
		ec.displayDetailedStats()
	case "q", "quit":
		return fmt.Errorf("quit")
	case "":
		// Empty command, just continue
		return nil
	default:
		ec.cli.printError(fmt.Sprintf("Unknown command: %s (type 'h' for help)", command))
	}
	return nil
}

// displayCommands shows available interactive commands
func (ec *EngineController) displayCommands() {
	fmt.Println("\nAvailable commands:")
	fmt.Println("  h, help    - Show this help")
	fmt.Println("  s, status  - Show current status")
	fmt.Println("  start      - Manually start the run")
	fmt.Println("  split      - Manually trigger a split")
	fmt.Println("  reset      - Reset the run")
	fmt.Println("  pause      - Pause the run")
	fmt.Println("  resume     - Resume the run")
	fmt.Println("  test       - Test current split condition")
	fmt.Println("  stats      - Show detailed statistics")
	fmt.Println("  q, quit    - Exit interactive mode")
}

// displayEngineStatus shows the current engine status
func (ec *EngineController) displayEngineStatus() {
	stats := ec.engine.GetStats()

	fmt.Println("\n" + strings.Repeat("─", 60))
	fmt.Printf("Engine Status: %s\n", stats.State.String())
	fmt.Printf("Run: %s (%s)\n", stats.RunName, stats.GameName)

	if stats.State == engine.StateRunning || stats.State == engine.StatePaused {
		fmt.Printf("Progress: %d/%d splits (%.1f%%)\n", stats.CurrentSplit, stats.TotalSplits, stats.Progress)
		if stats.CurrentSplitName != "" {
			fmt.Printf("Current Split: %s\n", stats.CurrentSplitName)
		}
		fmt.Printf("Elapsed Time: %v\n", stats.ElapsedTime.Truncate(time.Millisecond))
		if stats.TimeSinceLastSplit > 0 {
			fmt.Printf("Time Since Last Split: %v\n", stats.TimeSinceLastSplit.Truncate(time.Millisecond))
		}
	}
	fmt.Println(strings.Repeat("─", 60))
}

// displayDetailedStats shows detailed engine statistics
func (ec *EngineController) displayDetailedStats() {
	stats := ec.engine.GetStats()
	session := ec.engine.GetSession()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("DETAILED STATISTICS")
	fmt.Println(strings.Repeat("=", 60))

	// Basic info
	fmt.Printf("Run Configuration: %s\n", stats.RunName)
	fmt.Printf("Game: %s\n", stats.GameName)
	fmt.Printf("Engine State: %s\n", stats.State.String())
	fmt.Printf("Engine Running: %v\n", stats.IsRunning)
	fmt.Printf("Total Splits: %d\n", stats.TotalSplits)

	// Progress info
	if stats.State == engine.StateRunning || stats.State == engine.StatePaused || stats.State == engine.StateCompleted {
		fmt.Printf("\nProgress Information:\n")
		fmt.Printf("  Current Split: %d/%d\n", stats.CurrentSplit, stats.TotalSplits)
		fmt.Printf("  Progress: %.1f%%\n", stats.Progress)
		if stats.CurrentSplitName != "" {
			fmt.Printf("  Current Split Name: %s\n", stats.CurrentSplitName)
		}

		// Timing info
		fmt.Printf("\nTiming Information:\n")
		fmt.Printf("  Elapsed Time: %v\n", stats.ElapsedTime.Truncate(time.Millisecond))
		if stats.TimeSinceLastSplit > 0 {
			fmt.Printf("  Time Since Last Split: %v\n", stats.TimeSinceLastSplit.Truncate(time.Millisecond))
		}

		// Split states
		fmt.Printf("\nSplit States:\n")
		for i, splitName := range session.GetRunConfig().Splits {
			splitState := session.GetSplitState(splitName)
			status := "Pending"
			if splitState.IsCompleted() {
				status = "Completed"
			} else if i == stats.CurrentSplit && splitState.GetNextStep() > 0 {
				status = fmt.Sprintf("In Progress (Step %d)", splitState.GetNextStep())
			} else if i == stats.CurrentSplit {
				status = "Current"
			}
			fmt.Printf("  %2d. %-20s %s\n", i+1, splitName, status)
		}
	}

	// Autostart info
	if session.GetGameConfig().Autostart != nil {
		fmt.Printf("\nAutostart Configuration:\n")
		autostart := session.GetGameConfig().Autostart
		fmt.Printf("  Enabled: %s\n", autostart.Active)
		fmt.Printf("  Address: %s\n", autostart.Address)
		fmt.Printf("  Value: %s\n", autostart.Value)
		fmt.Printf("  Type: %s\n", autostart.Type)
		if autostart.Note != "" {
			fmt.Printf("  Note: %s\n", autostart.Note)
		}
	}

	fmt.Println(strings.Repeat("=", 60))
}

// handleManualStart handles manual start command
func (ec *EngineController) handleManualStart() error {
	if err := ec.engine.ManualStart(); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}
	ec.cli.printSuccess("Run started manually")
	return nil
}

// handleManualSplit handles manual split command
func (ec *EngineController) handleManualSplit() error {
	if err := ec.engine.ManualSplit(); err != nil {
		return fmt.Errorf("failed to split: %w", err)
	}
	ec.cli.printSuccess("Split triggered manually")
	return nil
}

// handleManualReset handles manual reset command
func (ec *EngineController) handleManualReset() error {
	if err := ec.engine.ManualReset(); err != nil {
		return fmt.Errorf("failed to reset: %w", err)
	}
	ec.cli.printSuccess("Run reset")
	return nil
}

// handlePause handles pause command
func (ec *EngineController) handlePause() error {
	if err := ec.engine.PauseEngine(); err != nil {
		return fmt.Errorf("failed to pause: %w", err)
	}
	ec.cli.printSuccess("Run paused")
	return nil
}

// handleResume handles resume command
func (ec *EngineController) handleResume() error {
	if err := ec.engine.ResumeEngine(); err != nil {
		return fmt.Errorf("failed to resume: %w", err)
	}
	ec.cli.printSuccess("Run resumed")
	return nil
}

// handleTestCondition tests the current split condition
func (ec *EngineController) handleTestCondition() error {
	split, splitState, err := ec.engine.GetCurrentSplitCondition()
	if err != nil {
		return fmt.Errorf("failed to get current split condition: %w", err)
	}

	ec.cli.printInfo(fmt.Sprintf("Testing condition for split: %s", split.Name))

	result, err := ec.engine.TestCondition(split)
	if err != nil {
		return fmt.Errorf("failed to test condition: %w", err)
	}

	if result {
		ec.cli.printSuccess("✓ Condition is currently TRUE")
	} else {
		ec.cli.printInfo("✗ Condition is currently FALSE")
	}

	// Show condition details
	fmt.Printf("Condition Details:\n")
	fmt.Printf("  Address: %s (0x%X)\n", split.Address, split.GetFxPakProAddress())
	fmt.Printf("  Value: %s (0x%X)\n", split.Value, split.ValueInt())
	fmt.Printf("  Type: %s\n", split.Type)

	if len(split.More) > 0 {
		fmt.Printf("  Additional Conditions (More): %d\n", len(split.More))
	}

	if len(split.Next) > 0 {
		fmt.Printf("  Sequential Conditions (Next): %d\n", len(split.Next))
		fmt.Printf("  Current Step: %d\n", splitState.GetNextStep())
	}

	return nil
}

// monitorEngineEvents monitors engine events and provides UI feedback
func (ec *EngineController) monitorEngineEvents(ctx context.Context) {
	splitChan := ec.engine.GetSplitChan()
	statusChan := ec.engine.GetStatusChan()

	for {
		select {
		case <-ctx.Done():
			return
		case splitEvent, ok := <-splitChan:
			if !ok {
				return
			}
			ec.handleSplitEvent(splitEvent)
		case statusEvent, ok := <-statusChan:
			if !ok {
				return
			}
			ec.handleStatusEvent(statusEvent)
		}
	}
}

// handleSplitEvent handles split events from the engine
func (ec *EngineController) handleSplitEvent(event engine.SplitEvent) {
	ec.cli.printSuccess(fmt.Sprintf("🚀 SPLIT: %s (Split %d)", event.SplitName, event.SplitIndex+1))

	// Show progress
	stats := ec.engine.GetStats()
	if stats.TotalSplits > 0 {
		fmt.Printf("Progress: %d/%d splits (%.1f%%) completed\n",
			event.SplitIndex+1, stats.TotalSplits, float64(event.SplitIndex+1)/float64(stats.TotalSplits)*100)
	}
}

// handleStatusEvent handles status events from the engine
func (ec *EngineController) handleStatusEvent(event engine.StatusEvent) {
	switch event.State {
	case engine.StateWaitingForStart:
		ec.cli.printInfo("⏳ " + event.Message)
	case engine.StateRunning:
		ec.cli.printSuccess("▶️ " + event.Message)
	case engine.StatePaused:
		ec.cli.printWarning("⏸️ " + event.Message)
	case engine.StateCompleted:
		ec.cli.printSuccess("🏁 " + event.Message)
	case engine.StateError:
		ec.cli.printError("❌ " + event.Message)
	default:
		ec.cli.printInfo("ℹ️ " + event.Message)
	}
}

// Callback implementations for engine events

func (ec *EngineController) onSplit(splitName string, splitIndex int) {
	// This is handled via the event channel monitoring
	ec.logger.WithFields(logrus.Fields{
		"split_name":  splitName,
		"split_index": splitIndex,
	}).Info("Split callback triggered")
}

func (ec *EngineController) onStart() {
	ec.logger.Info("Start callback triggered")
	// Additional start handling can be added here if needed
}

func (ec *EngineController) onReset() {
	ec.logger.Info("Reset callback triggered")
	// Additional reset handling can be added here if needed
}

func (ec *EngineController) onError(err error) {
	ec.logger.WithError(err).Error("Engine error callback triggered")
	ec.cli.printError(fmt.Sprintf("Engine error: %v", err))
}

func (ec *EngineController) onStateChange(oldState, newState engine.SplitterState) {
	ec.logger.WithFields(logrus.Fields{
		"old_state": oldState.String(),
		"new_state": newState.String(),
	}).Info("State change callback triggered")

	// Show special messages for important state changes
	switch newState {
	case engine.StateWaitingForStart:
		if oldState == engine.StateIdle {
			ec.cli.printInfo("Autostart enabled - waiting for start condition...")
			ec.cli.printInfo("The run will automatically start when the game begins")
		}
	case engine.StateRunning:
		if oldState == engine.StateWaitingForStart {
			ec.cli.printSuccess("🎮 Game started - run is now active!")
		}
	case engine.StateCompleted:
		stats := ec.engine.GetStats()
		ec.cli.printSuccess("🎉 RUN COMPLETED!")
		ec.cli.printSuccess(fmt.Sprintf("Final time: %v", stats.ElapsedTime.Truncate(time.Millisecond)))
		ec.cli.printInfo("Type 'reset' to start a new run")
	}
}

// GetEngine returns the underlying splitting engine (for external access)
func (ec *EngineController) GetEngine() *engine.SplittingEngine {
	return ec.engine
}

// IsInitialized returns whether the engine has been initialized
func (ec *EngineController) IsInitialized() bool {
	return ec.engine != nil
}

// ValidateConfiguration validates the engine configuration before initialization
func (ec *EngineController) ValidateConfiguration(runConfig *config.RunConfig, gameConfig *config.GameConfig) error {
	// Validate that all splits in the run exist in the game config
	for i, splitName := range runConfig.Splits {
		_, err := gameConfig.GetSplitByName(splitName)
		if err != nil {
			return fmt.Errorf("split '%s' at index %d not found in game config: %w", splitName, i, err)
		}
	}

	// Validate autostart configuration if present
	if gameConfig.Autostart != nil {
		if gameConfig.Autostart.Active == "1" {
			if gameConfig.Autostart.Address == "" {
				return fmt.Errorf("autostart is enabled but missing address")
			}
			if gameConfig.Autostart.Value == "" {
				return fmt.Errorf("autostart is enabled but missing value")
			}
			if gameConfig.Autostart.Type == "" {
				return fmt.Errorf("autostart is enabled but missing type")
			}
		}
	}

	return nil
}
