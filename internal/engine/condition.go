// internal/engine/condition.go
package engine

import (
	"context"
	"fmt"

	"github.com/jdharms/sni-autosplitter/internal/config"
	"github.com/jdharms/sni-autosplitter/internal/sni"
	"github.com/sirupsen/logrus"
)

// ConditionEvaluator handles evaluation of split conditions
type ConditionEvaluator struct {
	logger *logrus.Logger
	client *sni.Client
}

// NewConditionEvaluator creates a new condition evaluator
func NewConditionEvaluator(logger *logrus.Logger, client *sni.Client) *ConditionEvaluator {
	return &ConditionEvaluator{
		logger: logger,
		client: client,
	}
}

// EvaluateCondition evaluates a single split condition
func (ce *ConditionEvaluator) EvaluateCondition(ctx context.Context, deviceURI string, split *config.Split) (bool, error) {
	// Read memory value at the split's address
	memValue, err := ce.client.ReadMemoryValue(ctx, deviceURI, split.GetFxPakProAddress())
	if err != nil {
		return false, fmt.Errorf("failed to read memory at 0x%X: %w", split.GetFxPakProAddress(), err)
	}

	// Evaluate the condition based on type
	result := ce.evaluateComparison(split.Type, split.ValueInt(), memValue.ByteValue, memValue.WordValue)

	ce.logger.WithFields(logrus.Fields{
		"split":      split.Name,
		"address":    fmt.Sprintf("0x%X", split.GetFxPakProAddress()),
		"type":       split.Type,
		"expected":   fmt.Sprintf("0x%X", split.ValueInt()),
		"byte_value": fmt.Sprintf("0x%02X", memValue.ByteValue),
		"word_value": fmt.Sprintf("0x%04X", memValue.WordValue),
		"result":     result,
	}).Debug("Condition evaluated")

	return result, nil
}

// EvaluateComplexCondition evaluates a split with its More and Next conditions
func (ce *ConditionEvaluator) EvaluateComplexCondition(ctx context.Context, deviceURI string, split *config.Split, splitState *SplitState) (bool, error) {
	// Handle sequential conditions (Next)
	if len(split.Next) > 0 {
		return ce.evaluateNextConditions(ctx, deviceURI, split, splitState)
	}

	// Handle simultaneous conditions (More)
	if len(split.More) > 0 {
		return ce.evaluateMoreConditions(ctx, deviceURI, split)
	}

	// Simple condition
	return ce.EvaluateCondition(ctx, deviceURI, split)
}

// evaluateNextConditions handles sequential condition evaluation
func (ce *ConditionEvaluator) evaluateNextConditions(ctx context.Context, deviceURI string, split *config.Split, splitState *SplitState) (bool, error) {
	currentStep := splitState.NextStep

	// First check the main condition
	if currentStep == 0 {
		result, err := ce.EvaluateCondition(ctx, deviceURI, split)
		if err != nil {
			return false, err
		}

		if result {
			splitState.NextStep++
			ce.logger.WithFields(logrus.Fields{
				"split": split.Name,
				"step":  "main",
			}).Debug("Next condition step completed")
		}
		return false, nil // Don't trigger split on main condition
	}

	// Check the next condition at current step
	nextIndex := currentStep - 1
	if nextIndex >= len(split.Next) {
		// All steps completed - trigger split
		splitState.Reset()
		ce.logger.WithField("split", split.Name).Info("All next conditions completed - triggering split")
		return true, nil
	}

	nextSplit := &split.Next[nextIndex]
	result, err := ce.EvaluateCondition(ctx, deviceURI, nextSplit)
	if err != nil {
		return false, err
	}

	if result {
		splitState.NextStep++
		ce.logger.WithFields(logrus.Fields{
			"split": split.Name,
			"step":  nextIndex + 1,
			"total": len(split.Next),
		}).Debug("Next condition step completed")

		// Check if this was the final step
		if splitState.NextStep > len(split.Next) {
			splitState.Reset()
			ce.logger.WithField("split", split.Name).Info("All next conditions completed - triggering split")
			return true, nil
		}
	}

	return false, nil
}

// evaluateMoreConditions handles simultaneous condition evaluation
func (ce *ConditionEvaluator) evaluateMoreConditions(ctx context.Context, deviceURI string, split *config.Split) (bool, error) {
	// First evaluate the main condition
	mainResult, err := ce.EvaluateCondition(ctx, deviceURI, split)
	if err != nil {
		return false, err
	}

	if !mainResult {
		return false, nil
	}

	// All More conditions must be true
	for i, moreSplit := range split.More {
		moreResult, err := ce.EvaluateCondition(ctx, deviceURI, &moreSplit)
		if err != nil {
			return false, fmt.Errorf("failed to evaluate more condition %d: %w", i, err)
		}

		if !moreResult {
			ce.logger.WithFields(logrus.Fields{
				"split":          split.Name,
				"more_condition": i,
			}).Debug("More condition failed")
			return false, nil
		}
	}

	ce.logger.WithField("split", split.Name).Debug("All more conditions satisfied")
	return true, nil
}

// evaluateComparison performs the actual comparison based on type
func (ce *ConditionEvaluator) evaluateComparison(compType string, expectedValue uint32, byteValue uint8, wordValue uint16) bool {
	switch compType {
	case "eq":
		return uint32(byteValue) == expectedValue
	case "weq":
		return uint32(wordValue) == expectedValue
	case "bit":
		return (byteValue & uint8(expectedValue)) != 0
	case "wbit":
		return (wordValue & uint16(expectedValue)) != 0
	case "lt":
		return uint32(byteValue) < expectedValue
	case "wlt":
		return uint32(wordValue) < expectedValue
	case "lte":
		return uint32(byteValue) <= expectedValue
	case "wlte":
		return uint32(wordValue) <= expectedValue
	case "gt":
		return uint32(byteValue) > expectedValue
	case "wgt":
		return uint32(wordValue) > expectedValue
	case "gte":
		return uint32(byteValue) >= expectedValue
	case "wgte":
		return uint32(wordValue) >= expectedValue
	default:
		ce.logger.WithField("type", compType).Error("Unknown comparison type")
		return false
	}
}

// BatchEvaluateConditions evaluates multiple conditions efficiently using batch memory reads
func (ce *ConditionEvaluator) BatchEvaluateConditions(ctx context.Context, deviceURI string, splits []*config.Split) ([]bool, error) {
	if len(splits) == 0 {
		return []bool{}, nil
	}

	// Create batch memory reader
	batchReader := ce.client.NewBatchMemoryReader(deviceURI)

	// Add all memory requests
	addressMap := make(map[uint32]int) // address -> index in batch
	for _, split := range splits {
		address := split.GetFxPakProAddress()
		if _, exists := addressMap[address]; !exists {
			batchReader.AddWordRequest(address) // Read 2 bytes to get both byte and word values
			addressMap[address] = batchReader.RequestCount() - 1
		}

		// Add requests for More conditions
		for _, moreSplit := range split.More {
			moreAddress := moreSplit.GetFxPakProAddress()
			if _, exists := addressMap[moreAddress]; !exists {
				batchReader.AddWordRequest(moreAddress)
				addressMap[moreAddress] = batchReader.RequestCount() - 1
			}
		}

		// Add requests for Next conditions
		for _, nextSplit := range split.Next {
			nextAddress := nextSplit.GetFxPakProAddress()
			if _, exists := addressMap[nextAddress]; !exists {
				batchReader.AddWordRequest(nextAddress)
				addressMap[nextAddress] = batchReader.RequestCount() - 1
			}
		}
	}

	// Execute batch read
	memoryValues, err := batchReader.ExecuteAndParse(ctx)
	if err != nil {
		return nil, fmt.Errorf("batch memory read failed: %w", err)
	}

	// Create address to memory value map
	memValueMap := make(map[uint32]*sni.MemoryValue)
	for address, index := range addressMap {
		memValueMap[address] = memoryValues[index]
	}

	// Evaluate each split using cached memory values
	results := make([]bool, len(splits))
	for i, split := range splits {
		results[i] = ce.evaluateWithCachedValues(split, memValueMap)
	}

	return results, nil
}

// evaluateWithCachedValues evaluates a split using pre-read memory values
func (ce *ConditionEvaluator) evaluateWithCachedValues(split *config.Split, memValueMap map[uint32]*sni.MemoryValue) bool {
	// Get memory value for main condition
	memValue, exists := memValueMap[split.GetFxPakProAddress()]
	if !exists {
		ce.logger.WithField("address", fmt.Sprintf("0x%X", split.GetFxPakProAddress())).Error("Memory value not found in cache")
		return false
	}

	// Evaluate main condition
	mainResult := ce.evaluateComparison(split.Type, split.ValueInt(), memValue.ByteValue, memValue.WordValue)

	// Handle More conditions
	if len(split.More) > 0 {
		if !mainResult {
			return false
		}

		// All More conditions must be true
		for _, moreSplit := range split.More {
			moreMemValue, exists := memValueMap[moreSplit.GetFxPakProAddress()]
			if !exists {
				ce.logger.WithField("address", fmt.Sprintf("0x%X", moreSplit.GetFxPakProAddress())).Error("More condition memory value not found in cache")
				return false
			}

			moreResult := ce.evaluateComparison(moreSplit.Type, moreSplit.ValueInt(), moreMemValue.ByteValue, moreMemValue.WordValue)
			if !moreResult {
				return false
			}
		}
		return true
	}

	// For Next conditions, we only evaluate the main condition here
	// The sequential logic is handled elsewhere
	return mainResult
}
