package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jdharms/sni-autosplitter/internal/sni"
	"github.com/sirupsen/logrus"
)

// DeviceSelector handles the interactive device selection UI
type DeviceSelector struct {
	logger        *logrus.Logger
	deviceManager *sni.DeviceManager
	cli           *CLI
}

// NewDeviceSelector creates a new device selector
func NewDeviceSelector(logger *logrus.Logger, deviceManager *sni.DeviceManager, cli *CLI) *DeviceSelector {
	return &DeviceSelector{
		logger:        logger,
		deviceManager: deviceManager,
		cli:           cli,
	}
}

// SelectDevice presents an interactive device selection interface
func (ds *DeviceSelector) SelectDevice(ctx context.Context) (*sni.Device, error) {
	ds.cli.printInfo("Discovering SNI devices...")

	// Discover devices with timeout
	discoveryCtx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	devices, err := ds.deviceManager.DiscoverDevices(discoveryCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to discover devices: %w", err)
	}

	if len(devices) == 0 {
		ds.cli.printError("No compatible SNI devices found")
		ds.cli.printInfo("Make sure:")
		ds.cli.printInfo("  1. SNI is running (should be accessible at localhost:8191)")
		ds.cli.printInfo("  2. Your SNES device is connected and detected by SNI")
		ds.cli.printInfo("  3. The device supports memory reading")
		return nil, fmt.Errorf("no compatible devices found")
	}

	ds.cli.printSuccess(fmt.Sprintf("Found %d compatible device(s)", len(devices)))

	// If only one device, ask if user wants to use it
	if len(devices) == 1 {
		device := devices[0]
		ds.cli.printInfo(fmt.Sprintf("Found device: %s (%s)", device.DisplayName, device.Kind))

		if ds.askConfirmation("Use this device? (y/n): ") {
			return ds.testAndSelectDevice(ctx, device)
		} else {
			return nil, fmt.Errorf("user declined to use the only available device")
		}
	}

	// Multiple devices - show selection menu
	return ds.selectDeviceInteractive(ctx, devices)
}

// selectDeviceInteractive shows an interactive menu for device selection
func (ds *DeviceSelector) selectDeviceInteractive(ctx context.Context, devices []*sni.Device) (*sni.Device, error) {
	for {
		ds.cli.printInfo("Available SNI devices:")
		ds.listDevices(devices)

		// Get recommended device
		recommended := ds.deviceManager.GetRecommendedDevice(devices)
		if recommended != nil {
			for i, device := range devices {
				if device.URI == recommended.URI {
					ds.cli.printInfo(fmt.Sprintf("Recommended: %d (%s)", i+1, device.DisplayName))
					break
				}
			}
		}

		fmt.Print("\nSelect device (1-" + strconv.Itoa(len(devices)) + "), 'r' to refresh, or 'q' to quit: ")

		if !ds.cli.scanner.Scan() {
			return nil, fmt.Errorf("failed to read input")
		}

		input := strings.TrimSpace(ds.cli.scanner.Text())

		switch input {
		case "q", "quit":
			return nil, fmt.Errorf("user quit")
		case "r", "refresh":
			ds.cli.printInfo("Refreshing device list...")
			newDevices, err := ds.deviceManager.DiscoverDevices(ctx)
			if err != nil {
				ds.cli.printError(fmt.Sprintf("Failed to refresh devices: %v", err))
				continue
			}
			devices = newDevices
			ds.cli.printSuccess(fmt.Sprintf("Found %d device(s)", len(devices)))
			continue
		}

		choice, err := strconv.Atoi(input)
		if err != nil || choice < 1 || choice > len(devices) {
			ds.cli.printError(fmt.Sprintf("Invalid selection. Please enter a number between 1 and %d.", len(devices)))
			continue
		}

		selectedDevice := devices[choice-1]
		ds.cli.printSuccess(fmt.Sprintf("Selected: %s", selectedDevice.DisplayName))

		// Test the device before confirming
		device, err := ds.testAndSelectDevice(ctx, selectedDevice)
		if err != nil {
			ds.cli.printError(fmt.Sprintf("Device test failed: %v", err))
			if ds.askConfirmation("Try another device? (y/n): ") {
				continue
			} else {
				return nil, err
			}
		}

		return device, nil
	}
}

// testAndSelectDevice tests a device connection and returns it if successful
func (ds *DeviceSelector) testAndSelectDevice(ctx context.Context, device *sni.Device) (*sni.Device, error) {
	ds.cli.printInfo("Testing device connection...")

	// Test device connection with timeout
	testCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	err := ds.deviceManager.TestDeviceConnection(testCtx, device)
	if err != nil {
		return nil, fmt.Errorf("device connection test failed: %w", err)
	}

	ds.cli.printSuccess("Device connection test successful")
	ds.showDeviceInfo(device)

	return device, nil
}

// showDeviceInfo displays detailed information about a device
func (ds *DeviceSelector) showDeviceInfo(device *sni.Device) {
	ds.cli.printInfo("Device Information:")
	info := ds.deviceManager.GetDeviceInfo(device)
	for _, line := range strings.Split(info, "\n") {
		if line != "" {
			fmt.Printf("  %s\n", line)
		}
	}
}

// listDevices displays a numbered list of devices
func (ds *DeviceSelector) listDevices(devices []*sni.Device) {
	for i, device := range devices {
		capabilities := ds.getCapabilitiesString(device)
		fmt.Printf("  %d. %s (%s) - %s\n", i+1, device.DisplayName, device.Kind, capabilities)
	}
}

// getCapabilitiesString returns a formatted string of device capabilities
func (ds *DeviceSelector) getCapabilitiesString(device *sni.Device) string {
	var caps []string
	if device.SupportsMemoryRead {
		caps = append(caps, "Read")
	}
	if device.SupportsMemoryWrite {
		caps = append(caps, "Write")
	}
	if device.SupportsSystemReset {
		caps = append(caps, "Reset")
	}

	if len(caps) == 0 {
		return "No capabilities"
	}
	return strings.Join(caps, ", ")
}

// askConfirmation asks the user for a yes/no confirmation
func (ds *DeviceSelector) askConfirmation(prompt string) bool {
	for {
		fmt.Print(prompt)
		if !ds.cli.scanner.Scan() {
			return false
		}

		input := strings.ToLower(strings.TrimSpace(ds.cli.scanner.Text()))
		switch input {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		default:
			// default to true
			return true
		}
	}
}
