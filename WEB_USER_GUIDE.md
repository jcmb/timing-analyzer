# Timing Analyzer: Web Interface User Guide

The Web version of the Timing Analyzer is a multi-tenant cloud application. It allows you to connect to remote GNSS streams, NTRIP Casters, and Trimble IBSS networks directly from your web browser to monitor real-time packet latency and jitter.

## Starting a Session

When you navigate to the tool, you will be presented with the Setup form. Select your connection type to begin.

### Connection Types

1. **Direct TCP:** Connect to a raw, unauthenticated TCP port (e.g., a direct connection to a receiver's IP address).
   * **Required:** Host / IP Address, Port.

2. **Standard NTRIP:**
   Connect to an NTRIP v1 or v2 caster.
   * **Required:** Host / IP, Port, Mountpoint.
   * **Optional:** Username, Password (if the mountpoint requires authentication).

3. **Trimble IBSS:**
   A streamlined connection specific to Trimble Internet Base Station Service. The system will automatically route you to the correct server and format your credentials.
   * **Required:** Organization (Org), Mountpoint, Device Name, Password.
   * *Note:* The system automatically sets the Host to `<Org>.ibss.trimbleos.com` and formats your username as `<Device Name>.<Org>`.

### Analysis Settings
* **Expected Rate (Hz):** The frequency at which the base station or receiver is broadcasting data.
* **Allowed Jitter:** The acceptable deviation from the expected rate. Can be a percentage (`10%`) or an absolute time (`15ms`). Packets arriving outside this window will trigger a violation.
* **Payload Decoder:**
  * **None:** Analyzes the raw byte bursts.
  * **Trimble DCOL:** Extracts and categorizes individual DCOL message types.
  * **Moving Baseline (MB-CMR):** Extracts and categorizes MB-CMR payload structures.

## Reading the Dashboard

Once connected, you will see the Live Telemetry view.

* **Live Jitter Chart:** A real-time graph plotting the exact delta (latency) of incoming packets.
* **Stream Statistics Table:**
  * **Count:** Total number of packets received for that message type.
  * **Violations:** The number of times a packet arrived outside your configured jitter tolerance.
  * **Missed:** The number of completely dropped or skipped packets.
  * **Min/Max Δ:** The fastest and slowest arrival deltas recorded during the session.
* **Event Log:** A scrolling terminal showing the timestamp, packet type, and status of every processed packet. Connection errors (like `401 Unauthorized`) will appear here in red.

## Managing Your Session
* **Pause/Resume:** Click the "Live / Paused" indicator in the top right corner to freeze the charts so you can inspect a specific event. The server continues to process data in the background.
* **Stop & New Setup:** Click this button to completely terminate your connection to the remote stream and return to the Setup menu.
