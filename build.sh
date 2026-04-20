#!/bin/bash

# Create a bin directory for the compiled outputs
mkdir -p bin

echo "Building CLI and Server for Linux (Intel/AMD 64-bit)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-cli-linux-amd64 ./cmd/cli
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-server-linux-amd64 ./cmd/webserver

echo "Building CLI and Server for Linux ARM 32-bit (e.g., older Raspberry Pi)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-cli-linux-arm32 ./cmd/cli
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-server-linux-arm32 ./cmd/webserver

echo "Building CLI and Server for Linux ARM 64-bit (e.g., modern embedded/Raspberry Pi 4+)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-cli-linux-arm64 ./cmd/cli
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-server-linux-arm64 ./cmd/webserver

echo "Building CLI and Server for macOS (Apple Silicon / M-series)..."
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-cli-macos-arm64 ./cmd/cli
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-server-macos-arm64 ./cmd/webserver

echo "Building CLI and Server for macOS (Older Intel Macs)..."
env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-cli-macos-intel ./cmd/cli
env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-server-macos-intel ./cmd/webserver

echo "Building CLI and Server for Windows 64-bit..."
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-cli-windows-amd64.exe ./cmd/cli
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-server-windows-amd64.exe ./cmd/webserver

echo "Building CLI and Server for Windows 32-bit..."
env CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-cli-windows-386.exe ./cmd/cli
env CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer-server-windows-386.exe ./cmd/webserver

echo "Done! All executables are in the /bin folder."
