# Timing Analyzer: CLI User Guide

The Timing Analyzer Command Line Interface (CLI) is a lightweight, high-performance tool designed to monitor GNSS data streams and analyze packet timing jitter. It supports raw UDP/TCP streams, DCOL decoding, and MB-CMR parsing.

## Running the Application

Download the compiled executable for your operating system from the `/bin` folder. Run the application directly from your terminal or command prompt.

### Basic Syntax
```bash
./timing-analyzer-cli [flags]
```

## Command-Line Arguments

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--ip` | `udp` | Connection protocol: `tcp` or `udp`. |
| `--host` | `""` | Target IP or domain. If provided, implicitly forces `tcp` mode and makes the app act as a client. |
| `--port` | `2101` | The port to listen on (if acting as a server) or connect to. |
| `--decode` | `none` | Payload decoder to use. Options: `none`, `dcol`, `mb-cmr`. |
| `--rate` | `1.0` | Expected update rate of the stream in Hz. |
| `--jitter` | `10%` | Allowable jitter threshold. Can be a percentage (e.g., `10%`) or absolute milliseconds (e.g., `5ms`). |
| `--csv` | `""` | Output filename to save a detailed CSV log of all packets. |
| `--verbose` | `0` | Logging verbosity: `1` (shows warmup phase), `2` (shows all valid packets). |
| `--warmup` | `0` | Number of initial packets to ignore before enforcing jitter rules. |
| `--web-port`| `8080` | Local port to host the live interactive dashboard. |

## Examples

**1. Connecting to a Remote TCP Stream (Raw)**
Connect to a remote receiver at `192.168.1.50` on port `5017` expecting a 10Hz stream with a 5ms jitter tolerance.
```bash
./timing-analyzer-cli --host 192.168.1.50 --port 5017 --rate 10 --jitter 5ms
```

**2. Analyzing a Trimble DCOL Stream**
Connect to `sps855.com`, parse DCOL packets, expect a 1Hz rate, output a CSV file, and print all packets to the console.
```bash
./timing-analyzer-cli --host sps855.com --port 5017 --decode dcol --rate 1 --csv sps855_log.csv --verbose 2
```

**3. Listening for Incoming UDP Packets**
Open local port 30000, listen for incoming UDP packets, and analyze MB-CMR payload timing.
```bash
./timing-analyzer-cli --ip udp --port 30000 --decode mb-cmr --rate 1 --jitter 8%
```

## The Local Web Dashboard
Whenever the CLI is running, it automatically hosts a local web dashboard. Open your browser and navigate to `http://localhost:8080` to view real-time charts and stream statistics.
