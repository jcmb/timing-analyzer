# timing-analyzer

Network timing and jitter analyzer for GNSS and correction-data streams. Measure packet arrival deltas, detect missed packets, and monitor jitter against an expected rate.

## Binaries

| Command | Role |
| :--- | :--- |
| `cmd/cli` | Local tool with embedded or hub-mode web dashboard |
| `cmd/webserver` | Multi-tenant web UI (setup form + live SSE dashboard) |
| `cmd/gsof-dashboard` | GSOF stream statistics dashboard |
| `cmd/gsof-baseline` | Dual-stream GSOF baseline comparison |

Build everything (all OS/arch targets):

```bash
./build.sh
```

Outputs land under `bin/<application>/` (see `DEVELOPER_GUIDE.md`).

## Documentation

| Guide | Audience |
| :--- | :--- |
| `CLI_USER_GUIDE.md` | CLI flags, hub mode, local dashboard |
| `WEB_USER_GUIDE.md` | Web setup form, connection types, live dashboard |
| `GSOF_BASELINE_GUIDE.md` | GSOF baseline dual-stream setup and required GSOF message types |
| `DEVELOPER_GUIDE.md` | Architecture, build, webserver flags, reverse-proxy deployment |

## Web server quick start

```bash
./bin/webserver/server-linux-amd64 --bind=127.0.0.1 --port=2102 --base-path=/jitter
```

Open `http://127.0.0.1:2102/jitter/`. To pre-fill outbound Direct TCP on embedded devices:

```bash
./bin/webserver/server-linux-arm32 \
  --bind=127.0.0.1 --port=7001 --base-path=/jitter \
  --host=192.168.1.50 --stream-port=5018 --rate=1.0 --jitter=10% --decode=dcol
```

---

## Features

* **Multi-Protocol Support:** Supports both UDP and TCP protocols.
* **Flexible Modes:** Operates in either a server listening mode or a TCP client connection mode.
* **High-Precision Timing:** For UDP connections, it attempts to extract hardware or kernel-level timestamps (`SO_TIMESTAMP` / `SCM_TIMESTAMP`) from the operating system to ensure maximum accuracy.
* **OS Delay Tracking:** Calculates the delay between kernel reception and Go user-space wake-up.
* **Smart Jitter Compensation:** The timing engine suppresses transient jitter if it is compensated for by the offset of a previous packet.
* **Warmup Phase:** Includes a configurable warmup phase to ignore initial packets before enforcing jitter constraints.
* **Dead-Man Switch:** Exits with an error if no data is received within a duration equal to 100 expected update periods.

---

## 🛠️ Usage

To run the application, use the standard Go run or build commands, passing in your desired configuration flags:

```bash
# Run with default settings (UDP server on port 2101, 10Hz rate)
go run main.go

# Run as a TCP client connecting to a specific host
go run main.go -protocol tcp -host 192.168.1.50 -port 8080

# Run a UDP server with high verbosity and a custom jitter allowance
go run main.go -protocol udp -rate 50.0 -jitter 2.5 -verbose 2
```

### Configuration Flags

You can configure the application's parameters using the following command-line flags:

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-protocol` | `udp` | Specifies the protocol to use, either `tcp` or `udp`. |
| `-host` | `""` | An optional host IP to connect to when operating in TCP client mode. |
| `-port` | `2101` | The port the application will listen on or connect to. |
| `-rate` | `10.0` | The expected update rate of incoming packets, measured in Hz. |
| `-jitter` | `5.0` | The maximum allowable variance in packet arrival time, measured in milliseconds. |
| `-timeout-exit` | `true` | A boolean flag indicating whether the program should exit if no data arrives in 100 epochs. |
| `-verbose` | `0` | The logging verbosity level. |
| `-warmup` | `5` | The number of initial packets to ignore before calculating jitter violations. |

---

## 🏗️ Core Architecture

The application is built around two primary concurrent components communicating via a Go channel:

### 1. The Network Listener (`startListener`)
This component is responsible for handling raw network I/O based on the provided configuration. 
* **TCP Mode:** Binds to a port and accepts incoming connections, or dials a remote host with a 3-second timeout if a host IP is provided. 
* **UDP Mode:** Binds to a UDP address and reads messages while utilizing `syscall` control messages to parse out-of-band data for kernel timestamps. 
* **Packet Channel:** Every received network payload is wrapped in a `PacketEvent` struct containing the payload length, remote address, user-space time, and kernel time, which is then sent to a buffered channel capable of holding 1000 events.

### 2. The Timing Engine (`runTimingEngine`)
This component consumes events from the packet channel and performs mathematical validation against the expected update rate.
* **Delta Calculation:** Calculates the time delta between incoming packets using the most accurate timestamp available. 
* **Missed Packets:** If a time delta exceeds the threshold of two expected periods minus the allowable jitter, it calculates the estimated number of missed packets and logs an error.
* **Jitter Violations:** If a packet arrives outside the minimum or maximum expected time bounds, and the error wasn't compensated for by a previous packet, a jitter violation warning is logged.

---

## 📊 Logging and Verbosity

The application uses the structured `log/slog` package to output runtime information. Control the output volume using the `-verbose` flag:

* **Level 0 (Default):** Logs startup information, missed packets, jitter violations, connection losses, and critical errors.
* **Level 1:** Adds logs for the initial warmup phase and notifications when jitter is suppressed due to previous packet compensation.
* **Level 2:** Logs metadata for every single packet received, including OS delay microseconds, exact timestamps, and calculated delta times.

