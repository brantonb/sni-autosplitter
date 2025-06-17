package sni

import (
	"context"
	"fmt"
	"time"

	"github.com/jdharms/sni-autosplitter/pkg/sni"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the SNI gRPC client with connection management and retry logic
type Client struct {
	logger     *logrus.Logger
	address    string
	conn       *grpc.ClientConn
	devices    sni.DevicesClient
	memory     sni.DeviceMemoryClient
	control    sni.DeviceControlClient
	connected  bool
	retryDelay time.Duration
}

// Device represents an SNI device with its capabilities
type Device struct {
	URI                 string
	DisplayName         string
	Kind                string
	Capabilities        []sni.DeviceCapability
	DefaultAddressSpace sni.AddressSpace
	SupportsMemoryRead  bool
	SupportsMemoryWrite bool
	SupportsSystemReset bool
}

// NewClient creates a new SNI client
func NewClient(logger *logrus.Logger, host string, port int) *Client {
	address := fmt.Sprintf("%s:%d", host, port)
	return &Client{
		logger:     logger,
		address:    address,
		connected:  false,
		retryDelay: time.Second * 2,
	}
}

// Connect establishes a connection to the SNI server
func (c *Client) Connect(ctx context.Context) error {
	if c.connected {
		return nil
	}

	c.logger.WithField("address", c.address).Info("Connecting to SNI server")

	// Create connection with insecure credentials (SNI doesn't use TLS)
	conn, err := grpc.DialContext(ctx, c.address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to SNI at %s: %w", c.address, err)
	}

	c.conn = conn
	c.devices = sni.NewDevicesClient(conn)
	c.memory = sni.NewDeviceMemoryClient(conn)
	c.control = sni.NewDeviceControlClient(conn)
	c.connected = true

	c.logger.Info("Successfully connected to SNI server")
	return nil
}

// Disconnect closes the connection to the SNI server
func (c *Client) Disconnect() error {
	if !c.connected || c.conn == nil {
		return nil
	}

	c.logger.Info("Disconnecting from SNI server")
	err := c.conn.Close()
	c.connected = false
	c.conn = nil
	c.devices = nil
	c.memory = nil
	c.control = nil

	if err != nil {
		return fmt.Errorf("error closing SNI connection: %w", err)
	}

	return nil
}

// IsConnected returns whether the client is connected to SNI
func (c *Client) IsConnected() bool {
	return c.connected
}

// ConnectWithRetry attempts to connect with exponential backoff retry logic
func (c *Client) ConnectWithRetry(ctx context.Context, maxRetries int) error {
	var lastErr error
	delay := c.retryDelay

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c.logger.WithFields(logrus.Fields{
			"attempt":     attempt,
			"max_retries": maxRetries,
		}).Info("Attempting to connect to SNI")

		// Create a timeout context for this connection attempt
		connectCtx, cancel := context.WithTimeout(ctx, time.Second*10)
		err := c.Connect(connectCtx)
		cancel()

		if err == nil {
			return nil
		}

		lastErr = err
		c.logger.WithError(err).WithField("attempt", attempt).Warn("SNI connection attempt failed")

		if attempt < maxRetries {
			c.logger.WithField("delay", delay).Info("Waiting before retry")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			// Exponential backoff with jitter
			delay = time.Duration(float64(delay) * 1.5)
			if delay > time.Second*30 {
				delay = time.Second * 30
			}
		}
	}

	return fmt.Errorf("failed to connect to SNI after %d attempts: %w", maxRetries, lastErr)
}

// ListDevices discovers and returns available SNI devices
func (c *Client) ListDevices(ctx context.Context) ([]*Device, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected to SNI server")
	}

	c.logger.Debug("Listing SNI devices")

	resp, err := c.devices.ListDevices(ctx, &sni.DevicesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}

	devices := make([]*Device, 0, len(resp.Devices))
	for _, d := range resp.Devices {
		device := &Device{
			URI:                 d.Uri,
			DisplayName:         d.DisplayName,
			Kind:                d.Kind,
			Capabilities:        d.Capabilities,
			DefaultAddressSpace: d.DefaultAddressSpace,
		}

		// Check capabilities
		for _, cap := range d.Capabilities {
			switch cap {
			case sni.DeviceCapability_ReadMemory:
				device.SupportsMemoryRead = true
			case sni.DeviceCapability_WriteMemory:
				device.SupportsMemoryWrite = true
			case sni.DeviceCapability_ResetSystem:
				device.SupportsSystemReset = true
			}
		}

		devices = append(devices, device)
	}

	c.logger.WithField("count", len(devices)).Info("Discovered SNI devices")
	return devices, nil
}

// ReadMemory reads a single memory location from the specified device
func (c *Client) ReadMemory(ctx context.Context, deviceURI string, address uint32, size uint32) ([]byte, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected to SNI server")
	}

	request := &sni.SingleReadMemoryRequest{
		Uri: deviceURI,
		Request: &sni.ReadMemoryRequest{
			RequestAddress:      address,
			RequestAddressSpace: sni.AddressSpace_FxPakPro, // USB2SNES configs use FxPakPro address space
			Size:                size,
		},
	}

	resp, err := c.memory.SingleRead(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to read memory at 0x%X: %w", address, err)
	}

	c.logger.WithFields(logrus.Fields{
		"device":  deviceURI,
		"address": fmt.Sprintf("0x%X", address),
		"size":    size,
		"data":    fmt.Sprintf("0x%X", resp.Response.Data),
	}).Debug("Memory read successful")

	return resp.Response.Data, nil
}

// ReadMultipleMemory reads multiple memory locations from the specified device in a single request
func (c *Client) ReadMultipleMemory(ctx context.Context, deviceURI string, requests []MemoryRequest) ([][]byte, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected to SNI server")
	}

	if len(requests) == 0 {
		return [][]byte{}, nil
	}

	// Convert our requests to SNI requests
	sniRequests := make([]*sni.ReadMemoryRequest, len(requests))
	for i, req := range requests {
		sniRequests[i] = &sni.ReadMemoryRequest{
			RequestAddress:      req.Address,
			RequestAddressSpace: sni.AddressSpace_FxPakPro, // USB2SNES configs use FxPakPro address space
			Size:                req.Size,
		}
	}

	request := &sni.MultiReadMemoryRequest{
		Uri:      deviceURI,
		Requests: sniRequests,
	}

	resp, err := c.memory.MultiRead(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to read multiple memory locations: %w", err)
	}

	if len(resp.Responses) != len(requests) {
		return nil, fmt.Errorf("expected %d responses, got %d", len(requests), len(resp.Responses))
	}

	results := make([][]byte, len(resp.Responses))
	for i, response := range resp.Responses {
		results[i] = response.Data
	}

	c.logger.WithFields(logrus.Fields{
		"device":   deviceURI,
		"requests": len(requests),
	}).Debug("Multiple memory read successful")

	return results, nil
}

// ResetSystem resets the SNES system (if supported by the device)
func (c *Client) ResetSystem(ctx context.Context, deviceURI string) error {
	if !c.connected {
		return fmt.Errorf("not connected to SNI server")
	}

	request := &sni.ResetSystemRequest{
		Uri: deviceURI,
	}

	_, err := c.control.ResetSystem(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to reset system: %w", err)
	}

	c.logger.WithField("device", deviceURI).Info("System reset successful")
	return nil
}
