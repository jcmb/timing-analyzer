#!/bin/bash

# Ensure the bin directory exists for the compiled outputs
mkdir -p bin
mkdir -p bin/gsof


# Ensure the web directory exists (just in case)
mkdir -p web

echo "Downloading Chart.js for offline embedding..."
curl -s -o web/chart.umd.min.js https://cdn.jsdelivr.net/npm/chart.js@4.4.2/dist/chart.umd.min.js

if [ $? -ne 0 ]; then
    echo "Error: Failed to download Chart.js. Check your internet connection."
    exit 1
fi
echo "Chart.js downloaded successfully."
echo "------------------------------------------------"

mkdir -p bin/gsof

echo "Building GSOF for Linux (Intel/AMD 64-bit)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/gsof/gsof-linux-amd64 ./cmd/gsof-reporter

echo "Building GSOF for Linux ARM 32-bit (e.g., older Raspberry Pi)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/gsof/gsof-linux-arm32 ./cmd/gsof-reporter

echo "Building GSOF for Linux ARM 64-bit (e.g., modern embedded/Raspberry Pi 4+)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/gsof/gsof-linux-arm64 ./cmd/gsof-reporter

echo "Building GSOF for macOS (Apple Silicon / M-series)..."
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/gsof/gsof-macos-arm64 ./cmd/gsof-reporter

echo "Building GSOF for macOS (Older Intel Macs)..."
env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/gsof/gsof-macos-intel ./cmd/gsof-reporter

echo "Building GSOF for Windows 64-bit..."
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/gsof/gsof-windows-amd64.exe ./cmd/gsof-reporter

echo "Building GSOF for Windows 32-bit..."
env CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags="-w -s" -o bin/gsof/gsof-windows-386.exe ./cmd/gsof-reporter

echo "Done! All executables are in the /bin folder."
