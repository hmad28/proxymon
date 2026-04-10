# Proxymon

Proxymon is a Windows multi-ISP proxy monitor and traffic balancer with a system tray app and a real-time dashboard.

It binds outbound proxy traffic to selected network interfaces, lets you switch balancing modes from the tray, and shows both aggregate and per-interface traffic in a live dashboard.

## Features

- Windows system tray application
- WebView2-powered real-time dashboard when CGO is enabled
- Native dashboard fallback when CGO is disabled
- HTTP CONNECT proxy plus SOCKS5 proxy endpoint
- Multi-interface balancing with round-robin and failover modes
- Real NIC traffic monitoring per interface using Windows OS counters
- Live per-interface upload/download rates and totals
- Wi-Fi SSID display for wireless adapters
- Ethernet adapter/network description display with gateway context
- Windows proxy auto-enable option
- Resettable traffic totals from the dashboard and tray

## Requirements

- Windows 10 or later
- Go 1.25+
- MinGW-w64 in PATH for CGO/WebView builds
- WebView2 runtime installed on the machine for the WebView dashboard

## Installation and Build

### 1. Install MinGW-w64

If you use Scoop:

```bash
scoop install mingw
```

Then ensure MinGW is on PATH:

```bash
export PATH="/c/Users/Pongo/scoop/apps/mingw/current/bin:$PATH"
```

### 2. Build with WebView dashboard enabled

```bash
export PATH="/c/Users/Pongo/scoop/apps/mingw/current/bin:$PATH"
CGO_ENABLED=1 CC=gcc CXX=g++ go build -ldflags "-H=windowsgui" -o proxymon.exe ./cmd
```

### 3. Build native fallback without CGO

```bash
CGO_ENABLED=0 go build -ldflags "-H=windowsgui" -o proxymon.exe ./cmd
```

### 4. Validate the build

```bash
export PATH="/c/Users/Pongo/scoop/apps/mingw/current/bin:$PATH"
CGO_ENABLED=1 CC=gcc CXX=g++ go vet ./...
CGO_ENABLED=0 go vet ./...
```

## Usage

### Start Proxymon

```bash
./proxymon.exe
```

### Tray controls

After launch, Proxymon runs in the Windows system tray.

- **Left-click** the tray icon to open or focus the dashboard
- **Right-click** the tray icon to open the tray menu
- Use the tray menu to:
  - choose active interfaces
  - switch between round-robin and failover
  - enable or disable Windows auto-proxy
  - reset traffic stats
  - quit the app

### Dashboard

The dashboard shows:

- current proxy status
- total upload and download throughput
- last 60 seconds of traffic history
- proxy endpoints and connection counts
- uptime and version
- per-interface traffic totals and rates
- Wi-Fi SSID or Ethernet network/adapter description
- latest runtime errors

### Version output

```bash
./proxymon.exe -version
```

## Screenshot

_Screenshot placeholder: add tray + dashboard screenshot here._

## Notes

- `proxymon.exe` is ignored by git and should not be committed.
- The CGO build gives you the WebView dashboard.
- The non-CGO build keeps the tray app working with the native fallback dashboard.
