package sni

import (
	"context"
	"fmt"
)

// MemoryRequest represents a single memory read request
type MemoryRequest struct {
	Address uint32
	Size    uint32
}

// MemoryValue represents the result of reading memory
type MemoryValue struct {
	ByteValue uint8
	WordValue uint16
	Data      []byte
}

// ReadByte reads a single byte from memory
func (c *Client) ReadByte(ctx context.Context, deviceURI string, address uint32) (uint8, error) {
	data, err := c.ReadMemory(ctx, deviceURI, address, 1)
	if err != nil {
		return 0, err
	}

	if len(data) != 1 {
		return 0, fmt.Errorf("expected 1 byte, got %d", len(data))
	}

	return data[0], nil
}

// ReadWord reads a 16-bit word from memory (little-endian)
func (c *Client) ReadWord(ctx context.Context, deviceURI string, address uint32) (uint16, error) {
	data, err := c.ReadMemory(ctx, deviceURI, address, 2)
	if err != nil {
		return 0, err
	}

	if len(data) != 2 {
		return 0, fmt.Errorf("expected 2 bytes, got %d", len(data))
	}

	// SNES is little-endian
	return uint16(data[0]) | (uint16(data[1]) << 8), nil
}

// ReadMemoryValue reads memory and returns both byte and word interpretations
func (c *Client) ReadMemoryValue(ctx context.Context, deviceURI string, address uint32) (*MemoryValue, error) {
	// Read 2 bytes to get both byte and word values
	data, err := c.ReadMemory(ctx, deviceURI, address, 2)
	if err != nil {
		return nil, err
	}

	if len(data) < 1 {
		return nil, fmt.Errorf("no data returned")
	}

	value := &MemoryValue{
		ByteValue: data[0],
		Data:      data,
	}

	// If we have at least 2 bytes, calculate word value
	if len(data) >= 2 {
		value.WordValue = uint16(data[0]) | (uint16(data[1]) << 8)
	}

	return value, nil
}

// BatchMemoryReader helps efficiently read multiple memory locations
type BatchMemoryReader struct {
	client    *Client
	deviceURI string
	requests  []MemoryRequest
}

// NewBatchMemoryReader creates a new batch memory reader
func (c *Client) NewBatchMemoryReader(deviceURI string) *BatchMemoryReader {
	return &BatchMemoryReader{
		client:    c,
		deviceURI: deviceURI,
		requests:  make([]MemoryRequest, 0),
	}
}

// AddRequest adds a memory read request to the batch
func (bmr *BatchMemoryReader) AddRequest(address uint32, size uint32) {
	bmr.requests = append(bmr.requests, MemoryRequest{
		Address: address,
		Size:    size,
	})
}

// AddByteRequest adds a single byte read request
func (bmr *BatchMemoryReader) AddByteRequest(address uint32) {
	bmr.AddRequest(address, 1)
}

// AddWordRequest adds a single word read request
func (bmr *BatchMemoryReader) AddWordRequest(address uint32) {
	bmr.AddRequest(address, 2)
}

// Execute executes all batched memory read requests
func (bmr *BatchMemoryReader) Execute(ctx context.Context) ([][]byte, error) {
	if len(bmr.requests) == 0 {
		return [][]byte{}, nil
	}

	return bmr.client.ReadMultipleMemory(ctx, bmr.deviceURI, bmr.requests)
}

// ExecuteAndParse executes the batch and returns parsed memory values
func (bmr *BatchMemoryReader) ExecuteAndParse(ctx context.Context) ([]*MemoryValue, error) {
	results, err := bmr.Execute(ctx)
	if err != nil {
		return nil, err
	}

	values := make([]*MemoryValue, len(results))
	for i, data := range results {
		if len(data) == 0 {
			return nil, fmt.Errorf("no data returned for request %d", i)
		}

		value := &MemoryValue{
			ByteValue: data[0],
			Data:      data,
		}

		// If we have at least 2 bytes, calculate word value
		if len(data) >= 2 {
			value.WordValue = uint16(data[0]) | (uint16(data[1]) << 8)
		}

		values[i] = value
	}

	return values, nil
}

// Clear clears all pending requests
func (bmr *BatchMemoryReader) Clear() {
	bmr.requests = bmr.requests[:0]
}

// RequestCount returns the number of pending requests
func (bmr *BatchMemoryReader) RequestCount() int {
	return len(bmr.requests)
}
