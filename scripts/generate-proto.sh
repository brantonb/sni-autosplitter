#!/bin/bash
# Script to generate Go protobuf code from SNI proto file
# This script handles Go binary path issues and ensures proper installation

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Generating Go protobuf code from SNI proto...${NC}"

# Get Go binary path
GOBIN_PATH=$(go env GOPATH)/bin
if [ -n "$(go env GOBIN)" ]; then
    GOBIN_PATH=$(go env GOBIN)
fi

# Add Go bin to PATH if not already there
if [[ ":$PATH:" != *":$GOBIN_PATH:"* ]]; then
    echo -e "${BLUE}Adding Go bin directory to PATH: $GOBIN_PATH${NC}"
    export PATH="$PATH:$GOBIN_PATH"
fi

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo -e "${RED}Error: protoc is not installed${NC}"
    echo "Please install Protocol Buffers compiler:"
    echo "  - On Ubuntu: sudo apt-get install protobuf-compiler"
    echo "  - On macOS: brew install protobuf"
    echo "  - On Windows: Download from https://protobuf.dev/downloads/"
    exit 1
fi

# Function to install and verify Go protobuf plugins
install_go_plugin() {
    local plugin_name=$1
    local install_path=$2
    
    if ! command -v "$plugin_name" &> /dev/null; then
        echo -e "${YELLOW}Installing $plugin_name...${NC}"
        go install "$install_path"
        
        # Verify installation
        if ! command -v "$plugin_name" &> /dev/null; then
            echo -e "${RED}Error: $plugin_name installation failed or not in PATH${NC}"
            echo "Installed to: $GOBIN_PATH/$plugin_name"
            echo "Please ensure $GOBIN_PATH is in your PATH"
            exit 1
        else
            echo -e "${GREEN}$plugin_name installed successfully${NC}"
        fi
    else
        echo -e "${GREEN}$plugin_name is already installed${NC}"
    fi
}

# Install Go protobuf plugins
install_go_plugin "protoc-gen-go" "google.golang.org/protobuf/cmd/protoc-gen-go@latest"
install_go_plugin "protoc-gen-go-grpc" "google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"

# Check if proto file exists
if [ ! -f "sni.proto" ]; then
    echo -e "${RED}Error: sni.proto file not found in current directory${NC}"
    exit 1
fi

# Create output directory
mkdir -p pkg/sni

# Generate Go code from proto file
echo -e "${BLUE}Generating protobuf Go code...${NC}"
protoc \
    --go_out=pkg/sni \
    --go_opt=paths=source_relative \
    --go-grpc_out=pkg/sni \
    --go-grpc_opt=paths=source_relative \
    sni.proto

if [ $? -eq 0 ]; then
    echo -e "${GREEN}Successfully generated Go protobuf code in pkg/sni/${NC}"
    echo "Generated files:"
    echo "  - pkg/sni/sni.pb.go"
    echo "  - pkg/sni/sni_grpc.pb.go"
    
    # Show file sizes for verification
    if [ -f "pkg/sni/sni.pb.go" ]; then
        echo -e "${BLUE}File sizes:${NC}"
        ls -lh pkg/sni/sni*.pb.go
    fi
else
    echo -e "${RED}Error: Failed to generate protobuf code${NC}"
    exit 1
fi

echo -e "${GREEN}Done!${NC}"