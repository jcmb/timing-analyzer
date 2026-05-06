# Timing Analyzer: CLI User Guide

The Timing Analyzer command-line tool monitors packet timing and jitter on raw **TCP** or **UDP** streams. It can decode **DCOL** or **MB-CMR** payloads, optionally log to **CSV**, and always exposes a **local web dashboard** for charts and live telemetry.

Build artifacts live under `bin/cli/` (for example `cli-linux-amd64`, `cli-macos-arm64`, `cli-windows-amd64.exe`). You can also run from source:

```bash
go run ./cmd/cli [flags]
```

## Command-line flags

The CLI uses Go’s standard flag syntax: **one dash** (`-port`, `-hub=false`). Extra words after the flags are rejected (exit code 2), which helps catch typos such as `-hub falsse` left on the command line.

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-ip` | `tcp` | Transport for the embedded stream: `tcp` or `udp`. |
| `-udp` | `false` | Shorthand to set UDP mode (same as `-ip udp`). |
| `-host` | *(empty)* | Remote host for **TCP client** mode. If set, TCP is implied. |
| `-port` | `2101` | Port to listen on (UDP or TCP server) or to dial (TCP client when `-host` is set). |
| `-rate` | `1.0` | Expected stream rate in Hz. |
| `-jitter` | `10%` | Jitter tolerance: a percentage (e.g. `10%`) or milliseconds (e.g. `5ms`). |
| `-decode` | `none` | Payload decoder: `none`, `dcol`, or `mb-cmr`. |
| `-csv` | *(empty)* | If set, append `.csv` when needed and log packets to that file. |
| `-verbose` | `0` | `1` — warmup; `2` — every packet; `3` — parser debug. |
| `-warmup` | `0` | Number of initial packets to skip before enforcing jitter rules. |
| `-timeout-exit` | `true` | If true, exit with an error when no data is received for 100 expected periods. |
| `-web-port` | `8080` | HTTP port for the dashboard (and hub setup page). |
| `-web-host` | `127.0.0.1` | HTTP bind address. When hub layout is applied (see below), the CLI sets this to `0.0.0.0` so other machines can reach the setup page. |
| `-embedded-stream` | `true` | If true, run **one** stream from `-ip`/`-host`/`-port` and show it on the dashboard. If false, run **hub** mode (per-browser sessions). |
| `-hub` | `true` | Controls **hub shorthand** and **browser auto-open** (see below). Values: `true` / `false` and common aliases (`1`/`0`, `yes`/`no`, `on`/`off`). Invalid values fail at startup with a clear error. |

## Hub mode vs embedded (single) stream

**Embedded mode** (one stream from the CLI):

- The process listens or dials according to `-ip`, `-host`, and `-port`.
- The dashboard is at `http://<web-host>:<web-port>/` with Server-Sent Events at `/events`.

**Hub mode** (many concurrent sessions from the browser):

- The HTTP server listens on `-web-host` / `-web-port` (by default **all interfaces** when hub shorthand applies).
- Open `http://127.0.0.1:<web-port>/` (or your host) for the **setup** page. It shows the **CLI version** string baked into that build. Choose TCP or UDP and parameters, then **Connect** to open `/s/<session>/` for that session’s dashboard. Hub browser sessions use the same **`-verbose`** level as the CLI process that started the hub (there is no separate verbose control in the web form). SSE is at `/s/<session>/events`. When that SSE connection ends (for example **New connection** in the dashboard), the server tears down that session so the same port can be used again.

**When hub layout is applied** (multi-session UI, `-web-host` forced to `0.0.0.0` unless you only toggled `-hub`):

`-hub` is true **and** either you passed **`-hub`** on the command line, or you did **not** pass **`-embedded-stream`** **and** you did **not** set any of the **stream endpoint** flags: `-udp`, `-port`, `-host`, or `-ip`.

If you pass **any** of those stream flags **without** explicitly passing **`-hub`**, the CLI stays in **embedded** mode so flags such as `-udp -port 2101` run that stream directly instead of switching to the hub.

To get hub mode while still passing stream flags for defaults in the form, pass **`-hub`** explicitly (for example `./cli-linux-amd64 -hub -web-port 8080`).

## Browser auto-open

If **`-hub=true`** (the default), the CLI tries to open the default system browser to the dashboard URL after the HTTP listener is ready. The opened URL uses **127.0.0.1** when the server is bound to `0.0.0.0` so the link works on the same machine.

If **`-hub=false`**, the browser is **never** opened, regardless of other flags.

Failures to open the browser are logged as a warning; the process keeps running.

## Dashboard: command-line banner

If you pass **at least one** command-line flag other than **`-verbose`**, the HTML dashboard shows a **Command line** section: full process `argv` and a short table of resolved options (transport, host, port, rate, jitter, decode, verbose, warmup). With **no** flags (only the executable name), that section is omitted so the default hub view stays minimal.

When you start an **embedded** stream using any of **`-udp`**, **`-port`**, **`-host`**, or **`-ip`** on the command line, the process also prints the same `argv` and a `Web dashboard: http://…` line to **stdout**, and logs an **Embedded stream from CLI flags** line.

## Examples

**1. Remote TCP client**

```bash
./cli-linux-amd64 -host 192.168.1.50 -port 5017 -rate 10 -jitter 5ms
```

**2. DCOL decode, CSV log, verbose packets**

```bash
./cli-linux-amd64 -host sps855.com -port 5017 -decode dcol -rate 1 -csv sps855_log -verbose 2
```

**3. Local UDP listener (embedded stream from flags)**

```bash
./cli-linux-amd64 -udp -port 30000 -decode mb-cmr -rate 1 -jitter 8%
```

**4. Hub only (no stream flags): setup in the browser**

```bash
./cli-linux-amd64 -web-port 8080
```

**5. Headless embedded UDP (no browser window)**

```bash
./cli-linux-amd64 -hub=false -udp -port 2101
```

Open the printed `Web dashboard:` URL manually if you need the UI.

## See also

- `DEVELOPER_GUIDE.md` — layout, cross-build paths, and related binaries (`webserver`, etc.).
