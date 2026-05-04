#!/bin/bash

# Ensure the bin directory exists for the compiled outputs
mkdir -p bin
mkdir -p bin/timing-analyzer

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

echo "Building CLI and Server for Linux (Intel/AMD 64-bit)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/cli-linux-amd64 ./cmd/cli
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/server-linux-amd64 ./cmd/webserver

echo "Building CLI and Server for Linux ARM 32-bit (e.g., older Raspberry Pi)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/cli-linux-arm32 ./cmd/cli
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/server-linux-arm32 ./cmd/webserver

echo "Building CLI and Server for Linux ARM 64-bit (e.g., modern embedded/Raspberry Pi 4+)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/cli-linux-arm64 ./cmd/cli
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/server-linux-arm64 ./cmd/webserver

echo "Building CLI and Server for macOS (Apple Silicon / M-series)..."
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/cli-macos-arm64 ./cmd/cli
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/server-macos-arm64 ./cmd/webserver

echo "Building CLI and Server for macOS (Older Intel Macs)..."
env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/cli-macos-intel ./cmd/cli
env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/server-macos-intel ./cmd/webserver

echo "Building CLI and Server for Windows 64-bit..."
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/cli-windows-amd64.exe ./cmd/cli
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/server-windows-amd64.exe ./cmd/webserver

echo "Building CLI and Server for Windows 32-bit..."
env CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/cli-windows-386.exe ./cmd/cli
env CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/server-windows-386.exe ./cmd/webserver

echo "Building GSOF web dashboard (local SSE UI) for each target..."
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-dashboard-linux-amd64 ./cmd/gsof-dashboard
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-dashboard-linux-arm32 ./cmd/gsof-dashboard
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-dashboard-linux-arm64 ./cmd/gsof-dashboard
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-dashboard-macos-arm64 ./cmd/gsof-dashboard
env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-dashboard-macos-intel ./cmd/gsof-dashboard
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-dashboard-windows-amd64.exe ./cmd/gsof-dashboard
env CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-dashboard-windows-386.exe ./cmd/gsof-dashboard

echo "Building GSOF dual-stream baseline tool..."
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-baseline-linux-amd64 ./cmd/gsof-baseline
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-baseline-linux-arm32 ./cmd/gsof-baseline
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-baseline-linux-arm64 ./cmd/gsof-baseline
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-baseline-macos-arm64 ./cmd/gsof-baseline
env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-baseline-macos-intel ./cmd/gsof-baseline
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-baseline-windows-amd64.exe ./cmd/gsof-baseline
env CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags="-w -s" -o bin/timing-analyzer/gsof-baseline-windows-386.exe ./cmd/gsof-baseline

echo "Done! All executables are in the /bin folder."
