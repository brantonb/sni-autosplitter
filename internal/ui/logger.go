package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// InitializeLogger sets up file-based logging with the specified directory and level
func InitializeLogger(logDir, logLevel string) (*logrus.Logger, error) {
	logger := logrus.New()

	// Parse log level
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		return nil, fmt.Errorf("invalid log level '%s': %w", logLevel, err)
	}
	logger.SetLevel(level)

	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory '%s': %w", logDir, err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	logFile := filepath.Join(logDir, fmt.Sprintf("sni-autosplitter-%s.log", timestamp))

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file '%s': %w", logFile, err)
	}

	// Set up formatter
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})

	// Set output to file
	logger.SetOutput(file)

	logger.WithField("log_file", logFile).Info("Logger initialized")
	return logger, nil
}
