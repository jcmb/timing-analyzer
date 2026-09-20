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
Cross-compiled executables land under **`bin/<application>/`** (for example `bin/cli/cli-linux-amd64`, `bin/webserver/server-linux-amd64`, `bin/gsof-dashboard/…`, `bin/gsof-baseline/…`), each filename still including OS and architecture (and `.exe` on Windows).

| Application | Targets built |
| :--- | :--- |
| `cli` | linux-amd64, linux-arm32, linux-arm64, macos-arm64, windows-amd64 |
| `webserver` | linux-amd64, linux-arm32, linux-arm64, macos-arm64, windows-amd64 |
| `gsof-dashboard` | linux-amd64, linux-arm32, linux-arm64, macos-arm64, windows-amd64 |
| `gsof-baseline` | linux-amd64, linux-arm32, linux-arm64, macos-arm64, windows-amd64 |

## GSOF Baseline

`cmd/gsof-baseline` compares heading and reference GSOF streams for baseline attitude and range. See **`GSOF_BASELINE_GUIDE.md`** for which GSOF message types each receiver should output (heading vs moving-base stream, required vs optional).

## Web Server Command-Line Flags

The multi-tenant web server (`cmd/webserver`) exposes the setup form and session API. Run it directly or behind a reverse proxy.

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-port` | `2102` | HTTP listen port for the web UI and API. |
| `-bind` | `127.0.0.1` | Bind address. Use `0.0.0.0` to accept connections on all interfaces. |
| `-base-path` | `/` | URL prefix when mounted under a site path (e.g. `/jitter`). Handlers are registered under this prefix. |
| `-debug` | `false` | Log each incoming HTTP request (method, path, proxy headers) and non-success response status to stderr. |
| `-host` | *(empty)* | Pre-fill the setup form for **outbound Direct TCP** (host or IP). When set, defaults below are baked into the HTML. |
| `-stream-port` | `2101` | Pre-fill outbound TCP port (used with `-host`). |
| `-rate` | `1.0` | Pre-fill expected stream rate in Hz (used with `-host`). |
| `-jitter` | `10%` | Pre-fill allowable jitter, e.g. `10%` or `5ms` (used with `-host`). |
| `-decode` | `dcol` | Pre-fill payload decoder: `none`, `dcol`, or `mb-cmr` (used with `-host`). |

When `-host` is set, the server embeds those values in `index_server.html` as `SERVER_DEFAULT_SETUP`. They override any values saved in the browser’s `localStorage`, which is useful on embedded appliances where every user should see the same default TCP target. Users can still edit the form before connecting.

Example with preconfigured outbound TCP and request logging:

```bash
./bin/webserver/server-linux-arm64 \
  --bind=127.0.0.1 --port=7001 --base-path=/jitter \
  --host=192.168.1.50 --stream-port=5018 --rate=1.0 --jitter=10% --decode=dcol \
  --debug
```

## Web Server Lifecycle Management
The Web Server relies heavily on `context.Context` to prevent memory leaks.
When a user submits the Setup form, `handleStart` creates a dedicated `context.WithCancel()`. This context is passed into the `stream.StartNTRIPClient` and `timing.Run` goroutines.
When the user closes their browser tab or clicks "Stop", the SSE connection drops, triggering the `defer session.Cancel()` function in the HTTP router. This instantly tears down the network sockets and kills the goroutines for that specific user.

## Deployment (Reverse Proxy)

When deploying to a public-facing Linux server, the application should run behind a reverse proxy to handle HTTPS and sub-routing. The backend must receive requests **with** the `/jitter` prefix intact when `-base-path=/jitter` is set — do not strip the prefix in the proxy rewrite.

### 1. Systemd Service
Create a service at `/etc/systemd/system/timing-analyzer.service`:
```ini
[Unit]
Description=TrimbleTools Timing Analyzer
After=network.target

[Service]
User=your_username
WorkingDirectory=/home/your_username/
# --base-path must match the proxy location (e.g. /jitter)
# --bind 127.0.0.1 limits the HTTP port to localhost
# Optional: pre-fill outbound Direct TCP on the setup form
ExecStart=/home/your_username/bin/webserver/server-linux-amd64 \
  --port=7001 --base-path=/jitter --bind=127.0.0.1 \
  --host=192.168.1.50 --stream-port=5018 --rate=1.0 --jitter=10% --decode=dcol
Restart=always

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now timing-analyzer
journalctl -u timing-analyzer -f   # add --debug to ExecStart to trace proxied requests here
```

### 2. nginx Configuration

Proxy to the backend **without** rewriting away the base path. Example (`location` blocks in server context):

```nginx
location ~* ^/(jitter)$ {
    return 302 /jitter/;
}

location ~* ^/(jitter)/ {
    proxy_pass http://127.0.0.1:7001;
    proxy_http_version 1.1;
    proxy_connect_timeout 20s;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header X-Forwarded-Prefix /jitter;
}
```

Reload after testing: `sudo nginx -t && sudo nginx -s reload`.

### 3. Apache Configuration
Ensure `proxy`, `proxy_http`, and `headers` modules are enabled. Configure your VirtualHost to prevent SSE buffering:

```apache
<Location /jitter>
    ProxyPreserveHost On
    ProxyPass http://127.0.0.1:7001/jitter flushpackets=on
    ProxyPassReverse http://127.0.0.1:7001/jitter
</Location>

<Location /jitter/events>
    ProxyPreserveHost On

    # CRITICAL: Disable gzip to prevent SSE telemetry buffering
    SetEnv no-gzip 1
    SetEnv proxy-sendchunked 1
    ProxyTimeout 3600

    ProxyPass http://127.0.0.1:7001/jitter/events flushpackets=on
    ProxyPassReverse http://127.0.0.1:7001/jitter/events
</Location>
```
