package sni

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jdharms/sni-autosplitter/pkg/sni"
	"github.com/sirupsen/logrus"
)

// DeviceManager handles device discovery and selection
type DeviceManager struct {
	logger *logrus.Logger
	client *Client
}

// NewDeviceManager creates a new device manager
func NewDeviceManager(logger *logrus.Logger, client *Client) *DeviceManager {
	return &DeviceManager{
		logger: logger,
		client: client,
	}
}

// DiscoverDevices discovers all available SNI devices
func (dm *DeviceManager) DiscoverDevices(ctx context.Context) ([]*Device, error) {
	dm.logger.Info("Discovering SNI devices...")

	devices, err := dm.client.ListDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to discover devices: %w", err)
	}

	// Filter devices that support memory reading (required for autosplitting)
	var compatibleDevices []*Device
	for _, device := range devices {
		if device.SupportsMemoryRead {
			compatibleDevices = append(compatibleDevices, device)
		} else {
			dm.logger.WithFields(logrus.Fields{
				"device": device.DisplayName,
				"uri":    device.URI,
			}).Warn("Device does not support memory reading, skipping")
		}
	}

	// Sort devices by preference (FX Pak Pro first, then by display name)
	sort.Slice(compatibleDevices, func(i, j int) bool {
		a, b := compatibleDevices[i], compatibleDevices[j]

		// Prefer FX Pak Pro devices
		if a.Kind == "fxpakpro" && b.Kind != "fxpakpro" {
			return true
		}
		if b.Kind == "fxpakpro" && a.Kind != "fxpakpro" {
			return false
		}

		// Then sort by display name
		return a.DisplayName < b.DisplayName
	})

	dm.logger.WithField("count", len(compatibleDevices)).Info("Compatible devices discovered")

	for i, device := range compatibleDevices {
		dm.logger.WithFields(logrus.Fields{
			"index":        i + 1,
			"name":         device.DisplayName,
			"kind":         device.Kind,
			"uri":          device.URI,
			"memory_read":  device.SupportsMemoryRead,
			"memory_write": device.SupportsMemoryWrite,
			"reset":        device.SupportsSystemReset,
		}).Info("Discovered device")
	}

	return compatibleDevices, nil
}

// TestDeviceConnection tests if a device is working by attempting a simple memory read
func (dm *DeviceManager) TestDeviceConnection(ctx context.Context, device *Device) error {
	dm.logger.WithField("device", device.DisplayName).Info("Testing device connection")

	// Try to read a known SNES memory location (usually safe to read)
	// We'll read from the WRAM area which should always be accessible
	testAddress := uint32(0xF50000) // Start of WRAM in FxPakPro address space

	_, err := dm.client.ReadMemory(ctx, device.URI, testAddress, 1)
	if err != nil {
		return fmt.Errorf("device connection test failed: %w", err)
	}

	dm.logger.WithField("device", device.DisplayName).Info("Device connection test successful")
	return nil
}

// GetDeviceInfo returns detailed information about a device
func (dm *DeviceManager) GetDeviceInfo(device *Device) string {
	var info strings.Builder

	info.WriteString(fmt.Sprintf("Name: %s\n", device.DisplayName))
	info.WriteString(fmt.Sprintf("Type: %s\n", device.Kind))
	info.WriteString(fmt.Sprintf("URI: %s\n", device.URI))

	info.WriteString("Capabilities: ")
	var caps []string
	if device.SupportsMemoryRead {
		caps = append(caps, "Memory Read")
	}
	if device.SupportsMemoryWrite {
		caps = append(caps, "Memory Write")
	}
	if device.SupportsSystemReset {
		caps = append(caps, "System Reset")
	}

	if len(caps) > 0 {
		info.WriteString(strings.Join(caps, ", "))
	} else {
		info.WriteString("None")
	}

	return info.String()
}

// GetRecommendedDevice returns the most suitable device for autosplitting
func (dm *DeviceManager) GetRecommendedDevice(devices []*Device) *Device {
	if len(devices) == 0 {
		return nil
	}

	// Prefer FX Pak Pro devices as they have the most reliable memory access
	for _, device := range devices {
		if device.Kind == "fxpakpro" && device.SupportsMemoryRead {
			dm.logger.WithField("device", device.DisplayName).Info("Recommended device: FX Pak Pro")
			return device
		}
	}

	// Fall back to any device that supports memory reading
	for _, device := range devices {
		if device.SupportsMemoryRead {
			dm.logger.WithField("device", device.DisplayName).Info("Recommended device: First compatible device")
			return device
		}
	}

	return nil
}

// DetectMemoryMapping attempts to detect the memory mapping of the currently loaded ROM
func (dm *DeviceManager) DetectMemoryMapping(ctx context.Context, device *Device) (sni.MemoryMapping, error) {
	dm.logger.WithField("device", device.DisplayName).Info("Detecting memory mapping")

	if !dm.client.IsConnected() {
		return sni.MemoryMapping_Unknown, fmt.Errorf("not connected to SNI")
	}

	// Use SNI's built-in memory mapping detection
	request := &sni.DetectMemoryMappingRequest{
		Uri:                   device.URI,
		FallbackMemoryMapping: sni.MemoryMapping_LoROM, // Default to LoROM if detection fails
	}

	resp, err := dm.client.memory.MappingDetect(ctx, request)
	if err != nil {
		dm.logger.WithError(err).Warn("Failed to detect memory mapping using SNI")
		return sni.MemoryMapping_LoROM, fmt.Errorf("memory mapping detection failed: %w", err)
	}

	// Log the detection results
	if resp.Confidence {
		dm.logger.WithFields(logrus.Fields{
			"mapping":    resp.MemoryMapping.String(),
			"confidence": "high",
		}).Info("Successfully detected memory mapping")
	} else {
		dm.logger.WithFields(logrus.Fields{
			"mapping":    resp.MemoryMapping.String(),
			"confidence": "low",
		}).Warn("Memory mapping detection has low confidence, using fallback")
	}

	return resp.MemoryMapping, nil
}
