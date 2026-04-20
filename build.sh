#!/bin/bash
mkdir -p bin

echo "Building for Linux ARM 32-bit..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/jitter-tool-linux-arm32 ./cmd/cli

echo "Building for Linux ARM 64-bit..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/jitter-tool-linux-arm64 ./cmd/cli

echo "Building for macOS (Apple Silicon)..."
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/jitter-tool-macos-arm64 ./cmd/cli

echo "Building for Windows 64-bit..."
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/jitter-tool-windows-amd64.exe ./cmd/cli

echo "Done! All executables are in the /bin folder."
