# Proxymon

Proxymon is a Windows tray application that runs a local HTTP CONNECT and SOCKS5 proxy, binds outbound traffic to selected network interfaces, and shows live traffic through a dashboard.

It is built for multi-ISP and multi-adapter setups where you want to switch interfaces, choose balancing behavior, and monitor throughput from one place.

## Features

- Windows system tray app with left-click dashboard access and right-click quick actions
- HTTP CONNECT proxy with a companion SOCKS5 endpoint
- Round-robin and failover balancing modes
- Interface selection from the tray menu
- Windows auto-proxy toggle
- Live aggregate and per-interface traffic monitoring using Windows network counters
- WebView2 dashboard when CGO is enabled
- Native dashboard fallback when CGO is disabled
- Periodic interface rediscovery for Wi-Fi/IP changes and newly active adapters
- Automatic settings migration from the old Venn config path

## Requirements

- Windows 10 or later
- Go 1.25+
- WebView2 runtime for the WebView dashboard build
- MinGW-w64 in `PATH` if you want the CGO/WebView build

## Project layout

- `cmd/main.go` — application entrypoint, logging, startup, and shutdown handling
- `internal/app/` — runtime controller, snapshots, configuration application, hotplug refresh
- `internal/tray/` — systray app, menu actions, dashboard provider selection, popup errors
- `internal/dashboard/` — embedded WebView2 dashboard assets and bridge
- `internal/proxy/` — HTTP CONNECT and SOCKS5 proxy server
- `internal/netif/` — interface discovery, health checks, Windows traffic counters, Wi-Fi metadata
- `internal/winproxy/` — Windows proxy backup, enable, restore, and disable helpers
- `internal/config/` — persisted settings store
- `third_party/systray/` — local systray dependency override used by the project

## Configuration

Proxymon stores settings in:

```text
%AppData%/proxymon/settings.json
```

On first load it will migrate settings from the legacy path if this file exists:

```text
%AppData%/venn-combine-connection/settings.json
```

Persisted settings include:

- proxy listen address
- selected interfaces
- balancing mode
- Windows auto-proxy state

## Build

### Build with WebView2 dashboard

```bash
export PATH="/c/Users/Pongo/scoop/apps/mingw/current/bin:$PATH"
CGO_ENABLED=1 CC=gcc CXX=g++ go build -ldflags "-H=windowsgui" -o proxymon.exe ./cmd
```

### Build with native fallback dashboard

```bash
CGO_ENABLED=0 go build -ldflags "-H=windowsgui" -o proxymon.exe ./cmd
```

### Validate the codebase

```bash
go build ./...
```

## Run

### Start the app

```bash
./proxymon.exe
```

### Available flags

```bash
./proxymon.exe -addr 127.0.0.1:1080
./proxymon.exe -version
```

Default listen address:

```text
127.0.0.1:1080
```

The SOCKS5 listener runs on the next port, so the default SOCKS5 endpoint is `127.0.0.1:1081`.

## Tray behavior

After launch, Proxymon runs in the Windows tray.

- **Left-click** opens or focuses the dashboard
- **Right-click** opens the tray menu
- The tray menu lets you:
  - choose active interfaces
  - switch between round-robin and failover
  - enable or disable Windows auto-proxy
  - reset stats
  - quit the app

If only one interface is selected, Proxymon forces failover mode automatically.

## Dashboard

The dashboard shows:

- current proxy status
- current upload and download rate
- traffic history
- total uploaded and downloaded bytes
- active and total connection counts
- uptime and version
- active interface details, including IP, gateway, and network name
- latest runtime errors

## Notes

- `go build ./...` passes for the current repository state.
- The WebView dashboard is only built when CGO is enabled.
- The non-CGO build keeps the tray app working through the native dashboard fallback.
- The release workflow publishes `proxymon.exe` for tagged releases.
