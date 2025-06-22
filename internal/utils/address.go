package utils

import (
	"fmt"

	"github.com/jdharms/sni-autosplitter/pkg/sni"
)

// AddressConverter handles conversion between different SNES address spaces
type AddressConverter struct {
	memoryMapping sni.MemoryMapping
}

// NewAddressConverter creates a new address converter with the specified memory mapping
func NewAddressConverter(memoryMapping sni.MemoryMapping) *AddressConverter {
	return &AddressConverter{
		memoryMapping: memoryMapping,
	}
}

// IsValidFxPakProAddress checks if address is in FxPakPro space
func (ac *AddressConverter) IsValidFxPakProAddress(address uint32) bool {
	switch {
	case address <= 0xDFFFFF: // ROM (0x000000..0xDFFFFF)
		return true
	case address >= 0xE00000 && address <= 0xEFFFFF: // SRAM
		return true
	case address >= 0xF50000 && address <= 0xF6FFFF: // WRAM
		return true
	case address >= 0xF70000 && address <= 0xF7FFFF: // VRAM
		return true
	case address >= 0xF80000 && address <= 0xF8FFFF: // APU
		return true
	case address >= 0xF90000 && address <= 0xF901FF: // CGRAM
		return true
	case address >= 0xF90200 && address <= 0xF9041F: // OAM
		return true
	case address >= 0xF90420 && address <= 0xF904FF: // MISC
		return true
	case address >= 0xF90500 && address <= 0xF906FF: // PPUREG
		return true
	case address >= 0xF90700 && address <= 0xF908FF: // CPUREG
		return true
	case address >= 0x1000000 && address <= 0x1FFFFFF: // CMD space
		return true
	default:
		return false
	}
}

// ValidateFxPakProAddress validates that an address is in the valid FxPakPro address space
func (ac *AddressConverter) ValidateFxPakProAddress(address uint32) error {
	if ac.IsValidFxPakProAddress(address) {
		return nil
	}
	return fmt.Errorf("address 0x%06X is not in valid FxPakPro address space", address)
}

// GetAddressSpaceInfo returns information about what memory region an address belongs to
func (ac *AddressConverter) GetAddressSpaceInfo(address uint32) string {
	switch {
	case address <= 0xDFFFFF:
		return "ROM"
	case address >= 0xE00000 && address <= 0xEFFFFF:
		return "SRAM"
	case address >= 0xF50000 && address <= 0xF6FFFF:
		return "WRAM"
	case address >= 0xF70000 && address <= 0xF7FFFF:
		return "VRAM"
	case address >= 0xF80000 && address <= 0xF8FFFF:
		return "APU"
	case address >= 0xF90000 && address <= 0xF901FF:
		return "CGRAM"
	case address >= 0xF90200 && address <= 0xF9041F:
		return "OAM"
	case address >= 0xF90420 && address <= 0xF904FF:
		return "MISC"
	case address >= 0xF90500 && address <= 0xF906FF:
		return "PPUREG"
	case address >= 0xF90700 && address <= 0xF908FF:
		return "CPUREG"
	case address >= 0x1000000 && address <= 0x1FFFFFF:
		return "CMD"
	default:
		return "UNKNOWN"
	}
}

// IsWRAMAddress checks if an address is in the WRAM region (most common for game variables)
func (ac *AddressConverter) IsWRAMAddress(address uint32) bool {
	return address >= 0xF50000 && address <= 0xF6FFFF
}

// IsROMAddress checks if an address is in the ROM region
func (ac *AddressConverter) IsROMAddress(address uint32) bool {
	return address <= 0xDFFFFF
}

// IsSRAMAddress checks if an address is in the SRAM region
func (ac *AddressConverter) IsSRAMAddress(address uint32) bool {
	return address >= 0xE00000 && address <= 0xEFFFFF
}

// GetMemoryMapping returns the current memory mapping
func (ac *AddressConverter) GetMemoryMapping() sni.MemoryMapping {
	return ac.memoryMapping
}

// SetMemoryMapping updates the memory mapping
func (ac *AddressConverter) SetMemoryMapping(mapping sni.MemoryMapping) {
	ac.memoryMapping = mapping
}

// GetMemoryMappingName returns a human-readable name for the memory mapping
func (ac *AddressConverter) GetMemoryMappingName() string {
	switch ac.memoryMapping {
	case sni.MemoryMapping_LoROM:
		return "LoROM"
	case sni.MemoryMapping_HiROM:
		return "HiROM"
	case sni.MemoryMapping_ExHiROM:
		return "ExHiROM"
	case sni.MemoryMapping_SA1:
		return "SA1"
	case sni.MemoryMapping_Unknown:
		return "Unknown"
	default:
		return fmt.Sprintf("Unknown (%d)", ac.memoryMapping)
	}
}
