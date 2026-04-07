# timing-analyzer


# Network Timing and Jitter Analyzer



This application is a robust network monitoring tool designed to analyze packet arrival times, measure jitter, and detect dropped or missed packets. Operating as either a server or a client, it uses a precise timing engine to track network stability against expected data rates.

## 🚀 Features

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

---

Would you like me to help you draft some unit tests to validate the `runTimingEngine` logic, or perhaps generate a `Dockerfile` so you can easily containerize this tool?