#!/bin/bash

# Ensure the bin directory exists for the compiled outputs (one subdir per application)
mkdir -p bin
mkdir -p bin/cli bin/webserver bin/gsof-dashboard bin/gsof-baseline

# Ensure the web directory exists (just in case)
mkdir -p web

echo "Downloading Chart.js for offline embedding..."
curl -s -o web/chart.umd.min.js https://cdn.jsdelivr.net/npm/chart.js@4.4.2/dist/chart.umd.min.js
curl -s -o web/hammer.min.js https://cdn.jsdelivr.net/npm/hammerjs@2.0.8/hammer.min.js
curl -s -o web/chartjs-plugin-zoom.min.js https://cdn.jsdelivr.net/npm/chartjs-plugin-zoom@2.0.1/dist/chartjs-plugin-zoom.min.js

if [ $? -ne 0 ]; then
    echo "Error: Failed to download Chart.js assets. Check your internet connection."
    exit 1
fi
echo "Chart.js assets downloaded successfully."
echo "------------------------------------------------"

echo "Building CLI and Server for Linux (Intel/AMD 64-bit)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/cli/cli-linux-amd64 ./cmd/cli
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/webserver/server-linux-amd64 ./cmd/webserver

echo "Building CLI and Server for Linux ARM 32-bit (e.g., older Raspberry Pi)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/cli/cli-linux-arm32 ./cmd/cli
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/webserver/server-linux-arm32 ./cmd/webserver

echo "Building CLI and Server for Linux ARM 64-bit (e.g., modern embedded/Raspberry Pi 4+)..."
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/cli/cli-linux-arm64 ./cmd/cli
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/webserver/server-linux-arm64 ./cmd/webserver

echo "Building CLI and Server for macOS (Apple Silicon / M-series)..."
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/cli/cli-macos-arm64 ./cmd/cli
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/webserver/server-macos-arm64 ./cmd/webserver

echo "Building CLI and Server for Windows 64-bit..."
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/cli/cli-windows-amd64.exe ./cmd/cli
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/webserver/server-windows-amd64.exe ./cmd/webserver

echo "Building GSOF web dashboard (local SSE UI) for each target..."
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/gsof-dashboard/gsof-dashboard-linux-amd64 ./cmd/gsof-dashboard
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-w -s" -o bin/gsof-dashboard/gsof-dashboard-linux-arm32 ./cmd/gsof-dashboard
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/gsof-dashboard/gsof-dashboard-linux-arm64 ./cmd/gsof-dashboard
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-w -s" -o bin/gsof-dashboard/gsof-dashboard-macos-arm64 ./cmd/gsof-dashboard
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-w -s" -o bin/gsof-dashboard/gsof-dashboard-windows-amd64.exe ./cmd/gsof-dashboard

echo "Building GSOF dual-stream baseline tool..."
GSOF_BASELINE_LDFLAGS='-w -s'
if STAMP=$(date -u +%Y%m%d%H%M%S 2>/dev/null); then
  GSOF_BASELINE_LDFLAGS="$GSOF_BASELINE_LDFLAGS -X main.buildStamp=$STAMP"
fi
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$GSOF_BASELINE_LDFLAGS" -o bin/gsof-baseline/gsof-baseline-linux-amd64 ./cmd/gsof-baseline
env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="$GSOF_BASELINE_LDFLAGS" -o bin/gsof-baseline/gsof-baseline-linux-arm32 ./cmd/gsof-baseline
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="$GSOF_BASELINE_LDFLAGS" -o bin/gsof-baseline/gsof-baseline-linux-arm64 ./cmd/gsof-baseline
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="$GSOF_BASELINE_LDFLAGS" -o bin/gsof-baseline/gsof-baseline-macos-arm64 ./cmd/gsof-baseline
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="$GSOF_BASELINE_LDFLAGS" -o bin/gsof-baseline/gsof-baseline-windows-amd64.exe ./cmd/gsof-baseline

echo "Done! Outputs are under bin/<application>/ (cli, webserver, gsof-dashboard, gsof-baseline)."
