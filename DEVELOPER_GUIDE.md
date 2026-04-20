# Timing Analyzer: Developer & Deployment Guide

This document outlines the architecture, codebase structure, and deployment procedures for the TrimbleTools Timing Analyzer.

## Architecture Overview

The application is built in Go and utilizes a Session-Based Architecture to support multiple concurrent web users without leaking memory or blocking threads.
* **Core Engine:** A shared phase-locked loop mathematical engine (`internal/timing`) processes packet timestamps and enforces jitter rules.
* **CLI Target:** Wires the engine directly to OS standard output.
* **Web Target:** Uses a Session Manager and Server-Sent Events (SSE) to isolate engine instances per user, passing JSON telemetry to the browser.

## Project Structure

```text
timing-analyzer/
├── cmd/
│   ├── cli/main.go         # Entry point for the CLI tool
│   └── webserver/main.go   # Entry point for the multi-tenant web server
├── internal/
│   ├── core/types.go       # Shared structs (Config, LogEntry, Event limits)
│   ├── parser/dcol.go      # Binary decoding for DCOL/MB-CMR
│   ├── stream/             # Network dialers
│   │   ├── listener.go     # Raw TCP/UDP server/client (Used by CLI)
│   │   ├── ntrip.go        # HTTP/1.1 NTRIP v2 Client (Used by Web Server)
│   │   ├── sys_unix.go     # Hardware timestamping (Linux/macOS)
│   │   └── sys_windows.go  # Hardware timestamping stubs (Windows)
│   ├── telemetry/sse.go    # Server-Sent Events broker
│   └── timing/engine.go    # The core jitter and latency mathematical engine
└── web/
    ├── web.go              # go:embed directives
    ├── index.html          # CLI Dashboard UI
    └── index_server.html   # Web Server UI (Setup form + Dashboard)
```

## Building the Application

A bash script (`build.sh`) is provided to cross-compile the application for various architectures.

To build all binaries:
```bash
./build.sh
```
Executables will be deposited in the `/bin` folder, tagged with their respective OS, architecture, and target (`cli` vs `server`).

## Web Server Lifecycle Management
The Web Server relies heavily on `context.Context` to prevent memory leaks.
When a user submits the Setup form, `handleStart` creates a dedicated `context.WithCancel()`. This context is passed into the `stream.StartNTRIPClient` and `timing.Run` goroutines.
When the user closes their browser tab or clicks "Stop", the SSE connection drops, triggering the `defer session.Cancel()` function in the HTTP router. This instantly tears down the network sockets and kills the goroutines for that specific user.

## Deployment (Apache Reverse Proxy)

When deploying to a public-facing Linux server, the application should run behind a reverse proxy to handle HTTPS and sub-routing.

### 1. Systemd Service
Create a service at `/etc/systemd/system/timing-analyzer.service`:
```ini
[Unit]
Description=TrimbleTools Timing Analyzer
After=network.target

[Service]
User=your_username
WorkingDirectory=/home/your_username/
# Include --base-path if hosting on a sub-route
# Include --bind 127.0.0.1 to secure the port from external access
ExecStart=/home/your_username/timing-analyzer-server-linux-amd64 --port=8080 --base-path=/jitter --bind=127.0.0.1
Restart=always

[Install]
WantedBy=multi-user.target
```

### 2. Apache Configuration
Ensure `proxy`, `proxy_http`, and `headers` modules are enabled. Configure your VirtualHost to prevent SSE buffering:

```apache
<Location /jitter>
    ProxyPreserveHost On
    ProxyPass [http://127.0.0.1:8080/jitter](http://127.0.0.1:8080/jitter) flushpackets=on
    ProxyPassReverse [http://127.0.0.1:8080/jitter](http://127.0.0.1:8080/jitter)
</Location>

<Location /jitter/events>
    ProxyPreserveHost On

    # CRITICAL: Disable gzip to prevent SSE telemetry buffering
    SetEnv no-gzip 1
    SetEnv proxy-sendchunked 1
    ProxyTimeout 3600

    ProxyPass [http://127.0.0.1:8080/jitter/events](http://127.0.0.1:8080/jitter/events) flushpackets=on
    ProxyPassReverse [http://127.0.0.1:8080/jitter/events](http://127.0.0.1:8080/jitter/events)
</Location>
```
