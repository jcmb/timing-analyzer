#!/bin/bash

# Create a bin directory for the compiled outputs
mkdir -p bin

echo "Building for Linux ARM 32-bit (e.g., older Raspberry Pi)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/jitter-tool-linux-arm32 main.go

echo "Building for Linux ARM 64-bit (e.g., modern embedded/Raspberry Pi 4+)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/jitter-tool-linux-arm64 main.go

echo "Building for macOS (Apple Silicon / M-series)..."
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/jitter-tool-macos-arm64 main.go

#echo "Building for macOS (Older Intel Macs)..."
#env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/jitter-tool-macos-intel main.go

echo "Building for Windows 64-bit..."
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/jitter-tool-windows-amd64.exe main.go

#echo "Building for Windows 32-bit..."
#env CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags="-w -s" -o bin/jitter-tool-windows-386.exe main.go

echo "Done! All executables are in the /bin folder."
