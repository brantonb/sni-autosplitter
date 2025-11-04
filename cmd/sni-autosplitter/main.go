package main

import (
	"fmt"
	"os"

	"github.com/jdharms/sni-autosplitter/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// LiveSplitOnePort is the WebSocket port for LiveSplit One connections
	LiveSplitOnePort = 1990
	// MemoryPollInterval is the interval between memory reads in milliseconds
	MemoryPollInterval = 33
)

var (
	// Command line flags
	gamesDir        string
	runsDir         string
	logDir          string
	logLevel        string
	sniHost         string
	sniPort         int
	enableManualOps bool

	rootCmd = &cobra.Command{
		Use:   "sni-autosplitter [run-name]",
		Short: "SNI-based autosplitter for LiveSplit One",
		Long: `SNI AutoSplitter connects to SNI (Super Nintendo Interface) to read game memory
and automatically trigger splits in LiveSplit One based on configurable conditions.

The autosplitter uses a two-tier configuration system:
- Game configs define memory addresses and conditions
- Run configs define split sequences for specific categories

Examples:
  sni-autosplitter                    # Interactive mode - select from available runs
  sni-autosplitter "alttp any% nmg"  # Load specific run by name
  sni-autosplitter --games-dir ./custom/games --runs-dir ./custom/runs`,
		Args: cobra.MaximumNArgs(1),
		Run:  runAutosplitter,
	}
)

func init() {
	// Set up command line flags
	rootCmd.PersistentFlags().StringVar(&gamesDir, "games-dir", "./configs/games", "Directory containing game configuration files")
	rootCmd.PersistentFlags().StringVar(&runsDir, "runs-dir", "./configs/runs", "Directory containing run configuration files")
	rootCmd.PersistentFlags().StringVar(&logDir, "log-dir", "./logs", "Directory for log files")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&sniHost, "sni-host", "localhost", "SNI gRPC server host")
	rootCmd.PersistentFlags().IntVar(&sniPort, "sni-port", 8191, "SNI gRPC server port")
	rootCmd.PersistentFlags().BoolVar(&enableManualOps, "enable-manual-ops", false, "Enable manual split operations for development (split, reset, pause, resume, test)")

	// Bind flags to viper
	viper.BindPFlag("games-dir", rootCmd.PersistentFlags().Lookup("games-dir"))
	viper.BindPFlag("runs-dir", rootCmd.PersistentFlags().Lookup("runs-dir"))
	viper.BindPFlag("log-dir", rootCmd.PersistentFlags().Lookup("log-dir"))
	viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("sni-host", rootCmd.PersistentFlags().Lookup("sni-host"))
	viper.BindPFlag("sni-port", rootCmd.PersistentFlags().Lookup("sni-port"))
	viper.BindPFlag("enable-manual-ops", rootCmd.PersistentFlags().Lookup("enable-manual-ops"))
}

func runAutosplitter(cmd *cobra.Command, args []string) {
	// Initialize logger first
	logger, err := ui.InitializeLogger(logDir, logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	logger.Info("SNI AutoSplitter starting up")
	logger.WithFields(map[string]any{
		"games-dir":         gamesDir,
		"runs-dir":          runsDir,
		"log-dir":           logDir,
		"log-level":         logLevel,
		"sni-host":          sniHost,
		"sni-port":          sniPort,
		"enable-manual-ops": enableManualOps,
	}).Info("Configuration loaded")

	// Determine run name from args or interactive selection
	var runName string
	if len(args) > 0 {
		runName = args[0]
		logger.WithField("run", runName).Info("Run specified via command line")
	}

	// Create and start the CLI interface
	cliInterface := ui.NewCLI(logger, gamesDir, runsDir, enableManualOps, sniHost, sniPort)

	if err := cliInterface.Start(runName); err != nil {
		logger.WithError(err).Fatal("Failed to start autosplitter")
		os.Exit(1)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
