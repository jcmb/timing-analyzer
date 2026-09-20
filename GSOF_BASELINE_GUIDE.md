# GSOF Baseline User Guide

`gsof-baseline` compares a **heading receiver** stream to a **reference** position and reports baseline yaw, pitch, roll, horizontal/slant range, and optional checks against GSOF type 27. It accepts two independent DCOL/GSOF transports (TCP or UDP, including broadcast UDP on the listen port).

Build or run:

```bash
go run ./cmd/gsof-baseline/
# or
./bin/gsof-baseline/gsof-baseline-<os>-<arch>
```

Default mode is **hub**: open the web UI, configure heading and optional moving-base streams, then open a session dashboard. Use `-embedded-stream` and stream flags for a single fixed pair of transports from the CLI.

Official Trimble GSOF record descriptions: [GSOF messages overview](https://receiverhelp.trimble.com/oem-gnss/gsof-messages-overview.html).

---

## Wire format

Receivers must output **DCOL** with **GSOF** (packet type `0x40`). The tool decodes multi-page GSOF assemblies and expands **type 99** (extended records) before processing nested types ≥ 100.

Within each GSOF transmission, **type 1** (position time) must appear **before** **type 2** (LLH) so the rover epoch is paired with the correct GPS time-of-week (TOW).

---

## GSOF messages to output

The tables below use Trimble catalog numbers. “Required” means the dashboard cannot compute bearing/range without it. “Recommended” improves checks and status panels. “Optional / monitor” is shown when present but does not drive the baseline solution.

### Heading receiver stream

This is the rover / heading antenna stream. It is always required.

| GSOF type | Name | Role in gsof-baseline |
| :--- | :--- | :--- |
| **1** | Position time | **Required.** GPS TOW and SV count for each rover epoch. |
| **2** | Latitude, longitude, height | **Required.** Rover LLH; paired with the most recent type **1** in the same transmission. |
| **41** | Base position and quality | **Required for bearing** when used as the reference (preferred). TOW-matched to heading type **1** (default window 0.25 s). |
| **27** | Attitude info | **Recommended.** Yaw/pitch/roll and range; used for heading check (computed vs type 27) and optional slant range check vs type-27 range at the same TOW. |
| **15** | Receiver serial number | Optional. Shown in the serial panel. |
| **38** | Position type | Optional. Decoded position-type text for the heading receiver. |
| **35** | Received base station info | Optional. “Received base” on the heading stream; **not** used as the bearing reference (distinct from type **41**). |

**Minimum heading output for a solution:** types **1** + **2**, plus either:

- type **41** on this same stream, **or**
- a moving-base stream with types **1** + **2** (see below) when type **41** is not present here.

### Moving-base stream (optional second transport)

Enable when the physical base receiver is on a separate port or when heading type **41** is not available. TCP dial or UDP listen (including subnet / limited broadcast to the listen port) are supported.

| GSOF type | Name | Role in gsof-baseline |
| :--- | :--- | :--- |
| **1** | Position time | **Required** when this stream is the bearing reference (no heading type **41**). TOW-matched to heading type **1**. |
| **2** | Latitude, longitude, height | **Required** with type **1** when this stream is the bearing reference. |
| **35** | Received base station info | Optional / monitor. Shown in **Base station information** (preferred over type **41** on this stream). Not used for bearing. |
| **41** | Base position and quality | Optional / monitor. Shown in **Base station information** only if type **35** is absent. Not used for bearing. |
| **15** | Receiver serial number | Optional. Moving-base serial in the UI. |
| **38** | Position type | Optional. Position-type text for the base receiver. |
| **27** | Attitude info | Optional. Range pool for slant check only if heading type **27** is unavailable. |

**Bearing reference priority:**

1. Heading stream type **41** (when TOW-matched), then  
2. Moving-base stream types **1** + **2** (when TOW-matched).

Types **35** and **41** on the moving-base stream report **base station position for monitoring only**; they do not replace types **1** + **2** for bearing.

When heading stream type **41** is present, the UI can cross-check it against moving-base types **1** + **2** at a nearby TOW.

### Quality metrics (types 9 and 12)

When present, **type 9** (DOP) and **type 12** (position sigma) are paired with the latest **type 1** GPS TOW in each packet and shown in info cards plus time-series graphs (heading stream; moving-base stream when configured). PDOP, HDOP, TDOP, VDOP and σ East / North / Up / horizontal are plotted vs GPS TOW.

### Not used by gsof-baseline

Other GSOF types (3, 7, 8, 33, 34, 70, 97, 102, etc.) are ignored unless nested inside **type 99** as expanded children that match the types above. You do not need to disable them, but they do not affect the baseline dashboard.

---

## Computed outputs (from LLH)

For each matched heading epoch, the tool computes:

| Output | Source |
| :--- | :--- |
| Yaw | Horizontal azimuth from heading rover → reference (0–360°) |
| Pitch | Elevation from height difference and horizontal range |
| Roll | 0° (not fixed by two LLH positions alone) |
| Horizontal / slant range | Great-circle and slant distance to the reference |

Reference position is from heading type **41** or moving-base types **1** + **2**, not from moving-base types **35** / **41**.

---

## Command-line flags (summary)

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-heading-ip` | `tcp` | Heading transport: `tcp` or `udp`. |
| `-heading-host` | `172.27.0.14` | TCP dial host, or optional UDP bind IP (empty = all interfaces). |
| `-heading-port` | `6000` | TCP port or UDP listen port. |
| `-moving-base-ip` | `tcp` | Moving-base transport when `-moving-base-port` ≠ 0. |
| `-moving-base-host` | `172.27.0.14` | TCP dial host, or optional UDP bind IP. |
| `-moving-base-port` | `6001` | Moving-base port; `0` disables the second stream. |
| `-match-max-tow-delta-sec` | `0.25` | Max \|ΔTOW\| (s) between heading epoch and reference. |
| `-range-check-tolerance` | `0.01` | Slant range check tolerance (m); `0` disables. |
| `-expected-range` | `0` | Fixed slant reference (m); when `0`, uses type **27** range on heading when available. |
| `-hub` | `true` | Hub mode: configure streams in the browser (`-embedded-stream=false`, `-web-host=0.0.0.0`). |
| `-base-path` | (empty) | Public URL prefix behind a reverse proxy (e.g. `/baseline`). Must match `X-Forwarded-Prefix`. Env: `GSOF_BASELINE_BASE_PATH`. |
| `-web-port` | `8091` | HTTP port for UI and SSE. |

---

## Reverse proxy

Mount under a path prefix with `-base-path` (or `GSOF_BASELINE_BASE_PATH`). The prefix is baked into HTML links, API responses, and SSE URLs.

**Option A — forward the full path** (recommended):

```nginx
location ~* ^/(baseline)$ {
    return 302 /baseline/;
}

location ~* ^/(baseline)/ {
    proxy_pass http://127.0.0.1:7006;
    client_max_body_size 0;
    proxy_http_version 1.1;
    proxy_connect_timeout 20s;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
    proxy_buffering off;
    proxy_cache off;
    gzip off;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header Connection "";
    proxy_set_header X-Forwarded-Prefix /baseline;
}
```

Run: `gsof-baseline -base-path=/baseline -web-host=127.0.0.1 -web-port=7006 …`

**SSE requirements (nginx):**

- `proxy_buffering off` — without this, nginx holds the stream and the browser sees nothing
- `proxy_read_timeout 3600s` (or higher) — default 60s drops long-lived `/events` connections
- `gzip off` in this `location` — gzip on `text/event-stream` buffers chunks
- `proxy_set_header Connection ""` — do not use `Upgrade` here (that is for WebSockets, not SSE)
- Backend must receive `/baseline/…` paths **or** stripped paths with `X-Forwarded-Prefix: /baseline` (both are supported)

### Caddy (external HTTPS proxy)

`curl` can work while the **browser** spins on EventSource: browsers send `Accept-Encoding: gzip`, and Caddy’s default `encode gzip` **buffers** `text/event-stream` until the gzip block fills — `EventSource` gets headers (`onopen`) but no `data:` lines.

**Required on the `/baseline` route:** no `encode` handler (or `encode off`), and `flush_interval: -1` on `reverse_proxy`.

#### Caddyfile

```caddyfile
open.judo.kirk.dyndns.info {
    redir /baseline /baseline/ 302

    handle /baseline/* {
        encode off
        reverse_proxy 172.27.0.14:7006 {
            flush_interval -1
            header_up X-Forwarded-Prefix /baseline
            header_up X-Forwarded-Proto {scheme}
            header_up X-Forwarded-Host {host}
            transport http {
                read_timeout 0
                write_timeout 0
            }
        }
    }
}
```

#### Caddy JSON / Admin API (e.g. CaddyCfg)

Do **not** wrap `/baseline/*` in a site-wide `encode` handler. Example route fragment:

```json
{
  "@id": "baseline",
  "match": [{ "path": ["/baseline", "/baseline/*"] }],
  "handle": [
    {
      "handler": "reverse_proxy",
      "upstreams": [{ "dial": "172.27.0.14:7006" }],
      "flush_interval": -1,
      "headers": {
        "request": {
          "set": {
            "X-Forwarded-Prefix": ["/baseline"],
            "X-Forwarded-Proto": ["{http.request.scheme}"],
            "X-Forwarded-Host": ["{http.request.host}"]
          }
        }
      },
      "transport": {
        "protocol": "http",
        "read_timeout": 0,
        "write_timeout": 0
      }
    }
  ],
  "terminal": true
}
```

If your CaddyCfg template adds `encode` globally, add a **more specific** `/baseline/*` route (terminal, before encode) like above, or set `encodings` to skip `text/event-stream`.

Run backend: `gsof-baseline -base-path=/baseline -web-host=127.0.0.1 -web-port=7006 …`

**Alternative** — strip the prefix with `handle_path` (backend sees `/s/{id}/events`):

```caddyfile
handle_path /baseline/* {
    encode off
    reverse_proxy 127.0.0.1:7006 {
        flush_interval -1
        header_up X-Forwarded-Prefix /baseline
    }
}
```

`X-Forwarded-Prefix` is required when the path is stripped.

SSE URL: `/baseline/s/{session-id}/events` (same path tree as the dashboard).

**Symptoms**

| What you see | Likely cause |
| :--- | :--- |
| `curl` streams, browser Network shows 200 but no EventStream messages | Caddy `encode gzip` buffering |
| Header shows “SSE: connected”, msg count stays 0 | Caddy `encode gzip` buffering — fix proxy config (`encode off`, `flush_interval -1`) |
| `curl` works, msg count increases in header | fixed |

Verify through Caddy (session id from `/baseline/s/{id}`):

```bash
# curl often works without Accept-Encoding — mimic a browser:
curl -Nv -H 'Accept: text/event-stream' -H 'Accept-Encoding: gzip' \
  'https://open.judo.kirk.dyndns.info/baseline/s/YOUR_SESSION_ID/events'
```

If the gzip curl hangs or shows no `data:`, fix Caddy `encode` / `flush_interval`.

Direct backend (bypass proxy):

```bash
curl -N -H 'Accept: text/event-stream' 'http://127.0.0.1:7006/baseline/s/YOUR_SESSION_ID/events'
```

You should see `data: {"sse":"open"}` immediately, then `data: {…}` JSON every ~250 ms.

---

## Example receiver output checklist

**Heading receiver (minimum):**

- GSOF **1**, **2** every epoch  
- GSOF **41** on the same stream *or* rely on moving-base **1** + **2**

**Heading receiver (recommended):**

- Above, plus GSOF **27** (attitude / range for checks)

**Moving-base receiver (when used):**

- GSOF **1**, **2** if heading has no type **41**  
- GSOF **35** and/or **41** optional for the base-station information panel  
- UDP: aim broadcast or unicast at the hub listen port (e.g. `6001`); leave bind host empty for all interfaces

---

## Related documentation

| Guide | Content |
| :--- | :--- |
| `DEVELOPER_GUIDE.md` | Build, deployment, project layout |
| `README.md` | Repository overview and binaries |
| `cmd/gsof-dashboard` | Single-stream GSOF statistics (all subtypes) |
