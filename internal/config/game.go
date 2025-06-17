package config

import (
	"fmt"
	"strconv"
	"strings"
)

// GameConfig represents a game configuration file compatible with USB2SNES format
type GameConfig struct {
	Name        string  `json:"name,omitempty"` // Optional name field
	Game        string  `json:"game,omitempty"` // USB2SNES uses "game" field
	Autostart   *Split  `json:"autostart,omitempty"`
	Definitions []Split `json:"definitions"`
	// Note: We're not including IGT since SNES games use real-time
}

// Split represents a split condition with memory address, value, and comparison type
type Split struct {
	Name    string  `json:"name,omitempty"` // Name is optional for More/Next sub-splits
	Address string  `json:"address"`
	Value   string  `json:"value"`
	Type    string  `json:"type"`
	Note    string  `json:"note,omitempty"`
	More    []Split `json:"more,omitempty"`
	Next    []Split `json:"next,omitempty"`
	Active  string  `json:"active,omitempty"`

	// Cached parsed values (not serialized)
	addressInt uint32 `json:"-"`
	valueInt   uint32 `json:"-"`
	validated  bool   `json:"-"`
}

// ValidTypes contains all supported split condition types
var ValidTypes = map[string]bool{
	"eq":   true, // byte equal
	"weq":  true, // word equal
	"bit":  true, // byte bit set
	"wbit": true, // word bit set
	"lt":   true, // byte less than
	"wlt":  true, // word less than
	"lte":  true, // byte less or equal
	"wlte": true, // word less or equal
	"gt":   true, // byte greater than
	"wgt":  true, // word greater than
	"gte":  true, // byte greater or equal
	"wgte": true, // word greater or equal
}

// Validate validates the game configuration and all its splits
func (gc *GameConfig) Validate() error {
	// Check that we have either name or game field
	gameName := gc.GetGameName()
	if gameName == "" {
		return fmt.Errorf("game config missing both 'name' and 'game' fields")
	}

	// Validate autostart if present
	if gc.Autostart != nil {
		if err := gc.Autostart.validate("autostart", false); err != nil {
			return fmt.Errorf("autostart validation failed: %w", err)
		}
	}

	// Validate all definitions
	if len(gc.Definitions) == 0 {
		return fmt.Errorf("game config must have at least one split definition")
	}

	splitNames := make(map[string]bool)
	for i := range gc.Definitions {
		if err := gc.Definitions[i].validate(fmt.Sprintf("definition[%d]", i), true); err != nil {
			return fmt.Errorf("split '%s' validation failed: %w", gc.Definitions[i].Name, err)
		}

		// Check for duplicate split names (only for main definitions)
		if splitNames[gc.Definitions[i].Name] {
			return fmt.Errorf("duplicate split name '%s'", gc.Definitions[i].Name)
		}
		splitNames[gc.Definitions[i].Name] = true
	}

	return nil
}

// GetGameName returns the game name from either the "name" or "game" field
func (gc *GameConfig) GetGameName() string {
	if gc.Name != "" {
		return gc.Name
	}
	return gc.Game
}

// GetSplitByName returns the split definition with the given name
func (gc *GameConfig) GetSplitByName(name string) (*Split, error) {
	for i := range gc.Definitions {
		if gc.Definitions[i].Name == name {
			return &gc.Definitions[i], nil
		}
	}
	return nil, fmt.Errorf("split '%s' not found in game config", name)
}

// validate validates a split and all its sub-splits
func (s *Split) validate(context string, requireName bool) error {
	if s.validated {
		return nil // Already validated
	}

	// Only require name for main splits, not for More/Next sub-splits
	if requireName && s.Name == "" {
		return fmt.Errorf("%s: split missing required 'name' field", context)
	}

	if s.Address == "" {
		return fmt.Errorf("%s: split missing required 'address' field", context)
	}

	if s.Value == "" {
		return fmt.Errorf("%s: split missing required 'value' field", context)
	}

	if s.Type == "" {
		return fmt.Errorf("%s: split missing required 'type' field", context)
	}

	// Validate type
	if !ValidTypes[s.Type] {
		return fmt.Errorf("%s: split has invalid type '%s'", context, s.Type)
	}

	// Parse and validate address
	address, err := parseHexString(s.Address)
	if err != nil {
		return fmt.Errorf("%s: split has invalid address '%s': %w", context, s.Address, err)
	}
	s.addressInt = address

	// Parse and validate value
	value, err := parseHexString(s.Value)
	if err != nil {
		return fmt.Errorf("%s: split has invalid value '%s': %w", context, s.Value, err)
	}
	s.valueInt = value

	// Validate that word operations have appropriate values
	if strings.HasPrefix(s.Type, "w") && value > 0xFFFF {
		return fmt.Errorf("%s: split uses word operation but value 0x%X exceeds 16-bit range", context, value)
	}

	// Validate that byte operations have appropriate values
	if !strings.HasPrefix(s.Type, "w") && value > 0xFF {
		return fmt.Errorf("%s: split uses byte operation but value 0x%X exceeds 8-bit range", context, value)
	}

	// Validate 'more' and 'next' cannot both be present
	if len(s.More) > 0 && len(s.Next) > 0 {
		return fmt.Errorf("%s: split cannot have both 'more' and 'next' conditions", context)
	}

	// Validate 'more' conditions (names not required for sub-splits)
	for i := range s.More {
		moreContext := fmt.Sprintf("%s.more[%d]", context, i)
		if err := s.More[i].validate(moreContext, false); err != nil {
			return err
		}
		// 'more' splits cannot have nested 'more' or 'next'
		if len(s.More[i].More) > 0 || len(s.More[i].Next) > 0 {
			return fmt.Errorf("%s: nested 'more' or 'next' conditions are not supported", moreContext)
		}
	}

	// Validate 'next' conditions (names not required for sub-splits)
	for i := range s.Next {
		nextContext := fmt.Sprintf("%s.next[%d]", context, i)
		if err := s.Next[i].validate(nextContext, false); err != nil {
			return err
		}
		// 'next' splits can have 'more' but not nested 'next'
		if len(s.Next[i].Next) > 0 {
			return fmt.Errorf("%s: nested 'next' conditions are not supported", nextContext)
		}
	}

	s.validated = true
	return nil
}

// AddressInt returns the parsed address as uint32
func (s *Split) AddressInt() uint32 {
	return s.addressInt
}

// GetFxPakProAddress returns the address converted to FxPakPro space for SNI
func (s *Split) GetFxPakProAddress() uint32 {
	// Convert USB2SNES-style address to FxPakPro space
	// This adds 0xF50000 to SNES addresses to map them to WRAM
	if s.addressInt >= 0xF50000 {
		// Already in FxPakPro space
		return s.addressInt
	}

	// Convert to WRAM space (matches USB2SNES behavior)
	return 0xF50000 + s.addressInt
}

// ValueInt returns the parsed value as uint32
func (s *Split) ValueInt() uint32 {
	return s.valueInt
}

// parseHexString parses a hex string (with or without 0x prefix) into uint32
func parseHexString(hexStr string) (uint32, error) {
	// Remove 0x prefix if present
	hexStr = strings.TrimPrefix(hexStr, "0x")
	hexStr = strings.TrimPrefix(hexStr, "0X")

	if hexStr == "" {
		return 0, fmt.Errorf("empty hex string")
	}

	// Parse as hex
	value, err := strconv.ParseUint(hexStr, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid hex value: %w", err)
	}

	return uint32(value), nil
}
