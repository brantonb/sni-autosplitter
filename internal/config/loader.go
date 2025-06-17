package config

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// ConfigLoader handles loading and validation of game and run configurations
type ConfigLoader struct {
	logger   *logrus.Logger
	gamesDir string
	runsDir  string
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader(logger *logrus.Logger, gamesDir, runsDir string) *ConfigLoader {
	return &ConfigLoader{
		logger:   logger,
		gamesDir: gamesDir,
		runsDir:  runsDir,
	}
}

// LoadGameConfig loads and validates a game configuration by game name
func (cl *ConfigLoader) LoadGameConfig(gameName string) (*GameConfig, error) {
	gameFile := filepath.Join(cl.gamesDir, gameName+".json")

	cl.logger.WithField("file", gameFile).Debug("Loading game config")

	data, err := os.ReadFile(gameFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read game config file '%s': %w", gameFile, err)
	}

	var gameConfig GameConfig
	if err := json.Unmarshal(data, &gameConfig); err != nil {
		return nil, fmt.Errorf("failed to parse game config file '%s': %w", gameFile, err)
	}

	if err := gameConfig.Validate(); err != nil {
		return nil, fmt.Errorf("game config validation failed for '%s': %w", gameFile, err)
	}

	cl.logger.WithFields(logrus.Fields{
		"game":        gameConfig.Name,
		"definitions": len(gameConfig.Definitions),
	}).Info("Game config loaded successfully")

	return &gameConfig, nil
}

// LoadRunConfig loads and validates a run configuration by filename
func (cl *ConfigLoader) LoadRunConfig(filename string) (*RunConfig, error) {
	runFile := filepath.Join(cl.runsDir, filename)

	cl.logger.WithField("file", runFile).Debug("Loading run config")

	data, err := os.ReadFile(runFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read run config file '%s': %w", runFile, err)
	}

	var runConfig RunConfig
	if err := json.Unmarshal(data, &runConfig); err != nil {
		return nil, fmt.Errorf("failed to parse run config file '%s': %w", runFile, err)
	}

	if err := runConfig.Validate(); err != nil {
		return nil, fmt.Errorf("run config validation failed for '%s': %w", runFile, err)
	}

	cl.logger.WithFields(logrus.Fields{
		"run":      runConfig.Name,
		"category": runConfig.Category,
		"game":     runConfig.Game,
		"splits":   len(runConfig.Splits),
	}).Info("Run config loaded successfully")

	return &runConfig, nil
}

// DiscoverRuns scans the runs directory and loads all valid run configurations
func (cl *ConfigLoader) DiscoverRuns() ([]*RunConfig, error) {
	cl.logger.WithField("dir", cl.runsDir).Info("Discovering run configurations")

	if _, err := os.Stat(cl.runsDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("runs directory '%s' does not exist", cl.runsDir)
	}

	var runs []*RunConfig
	var errors []string

	err := filepath.WalkDir(cl.runsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-JSON files
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return nil
		}

		// Get relative filename
		relPath, err := filepath.Rel(cl.runsDir, path)
		if err != nil {
			cl.logger.WithError(err).WithField("path", path).Warn("Failed to get relative path")
			return nil
		}

		// Load the run config
		runConfig, err := cl.LoadRunConfig(relPath)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to load run config '%s': %v", relPath, err)
			errors = append(errors, errorMsg)
			cl.logger.WithError(err).WithField("file", relPath).Error("Run config validation failed")
			return nil // Continue processing other files
		}

		runs = append(runs, runConfig)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan runs directory: %w", err)
	}

	// If we have any validation errors, fail strictly
	if len(errors) > 0 {
		return nil, fmt.Errorf("run configuration validation failed:\n%s", strings.Join(errors, "\n"))
	}

	cl.logger.WithField("count", len(runs)).Info("Run discovery completed")
	return runs, nil
}

// FindRunByCategory finds a run configuration by category name
func (cl *ConfigLoader) FindRunByCategory(runs []*RunConfig, category string) (*RunConfig, error) {
	var matches []*RunConfig

	// Look for exact category matches
	for _, run := range runs {
		if strings.EqualFold(run.Category, category) {
			matches = append(matches, run)
		}
	}

	// If no exact matches, try partial matches
	if len(matches) == 0 {
		lowerCategory := strings.ToLower(category)
		for _, run := range runs {
			if strings.Contains(strings.ToLower(run.Category), lowerCategory) {
				matches = append(matches, run)
			}
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no run found matching category '%s'", category)
	}

	if len(matches) > 1 {
		var categories []string
		for _, match := range matches {
			categories = append(categories, match.Category)
		}
		return nil, fmt.Errorf("multiple runs found matching category '%s': %s", category, strings.Join(categories, ", "))
	}

	return matches[0], nil
}

// LoadRunAndGame loads both run and game configurations and validates they work together
func (cl *ConfigLoader) LoadRunAndGame(runConfig *RunConfig) (*RunConfig, *GameConfig, error) {
	// Load the game config
	gameConfig, err := cl.LoadGameConfig(runConfig.Game)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load game config for run '%s': %w", runConfig.Name, err)
	}

	// Validate that the run's splits exist in the game config
	if err := runConfig.ValidateAgainstGame(gameConfig); err != nil {
		return nil, nil, fmt.Errorf("run/game validation failed: %w", err)
	}

	cl.logger.WithFields(logrus.Fields{
		"run":  runConfig.Name,
		"game": gameConfig.Name,
	}).Info("Run and game configurations validated successfully")

	return runConfig, gameConfig, nil
}
