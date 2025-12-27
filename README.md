# GTA Vice City — HTML5 Port (DOS Zone)

Web-based port of GTA: Vice City running in browser via WebAssembly.

> **Fast Start:** `go run server.go` or `docker compose up -d --build`  then open http://localhost:8000

## Requirements

- Docker (recommended) or Go 1.25+

## Quick Start

1.  **Clone the repository**:
    ```bash
    git clone https://github.com/adambenhassen/reVCDOS.git
    cd reVCDOS
    ```

2. **Configure Assets** (Optional):

   By default, the project uses the **DOS Zone CDN**. For fully offline/local hosting, see [Offline Setup](#offline-setup) below.
4. **Launch the Application**:
   Choose one of the setup methods below:
   * **Docker** (Recommended for most users) — fast and isolated.
   * **Go** — Single binary, no dependencies.

## Setup & Running

### Option 1: Using Docker (Recommended)
The easiest way to get started is using Docker Compose:

```bash
docker compose up -d --build
```

To configure server options via environment variables:

```bash
# Enable auth and pre-download all assets
AUTH_LOGIN=admin AUTH_PASSWORD=secret DOWNLOAD_CACHE=1 docker compose up -d --build
```

| Environment Variable | Description |
|---------------------|-------------|
| `PORT` | Server port (default: 8000) |
| `AUTH_LOGIN` | HTTP Basic Auth username |
| `AUTH_PASSWORD` | HTTP Basic Auth password |
| `CDN` | Custom CDN base URL (default: `https://cdn.dos.zone/vcsky/`) |
| `DOWNLOAD_CACHE` | Download all assets in background on startup (set to `1` or `true`) |
| `WORKERS` | Number of parallel download workers (default: 8) |

### Option 2: Go Server (Recommended for local)

```bash
go run server.go
```

Server starts at `http://localhost:8000`. Assets are proxied from CDN and cached locally.

> **Note:** Use `go build -o server-go server.go` to create a single binary with all assets embedded.

## Server Options (Go)

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `-port` | int | 8000 | Server port |
| `-login` | string | none | HTTP Basic Auth username |
| `-password` | string | none | HTTP Basic Auth password |
| `-cdn` | string | `https://cdn.dos.zone/vcsky/` | CDN base URL |
| `-download` | flag | disabled | Download all assets and exit |
| `-download-cache` | flag | disabled | Download all assets in background while serving |
| `-workers` | int | 8 | Number of parallel download workers |

**Examples:**
```bash
# Start on custom port
go run server.go -port 3000

# Enable HTTP Basic Authentication
go run server.go -login admin -password secret123

# Download all assets for offline use
go run server.go -download

# Start server and download assets in background
go run server.go -download-cache

# Use custom CDN
go run server.go -cdn https://my-cdn.example.com/vcsky/
```

> **Note:** HTTP Basic Auth is only enabled when both `-login` and `-password` are provided.

> **Note:** Assets are proxied from CDN and cached locally in `vcsky/` directory. Use `-download` to pre-download all assets for fully offline play.

## URL Parameters

| Parameter | Values | Description |
|-----------|--------|-------------|
| `lang` | `en` | Game language |
| `cheats` | `1` | Enable cheat menu (F3) |
| `request_original_game` | `1` | Request original game files before play |
| `fullscreen` | `0` | Disable auto-fullscreen |
| `max_fps` | `1-240` | Limit frame rate (e.g., `60` for 60 FPS) |


**Example:**
- `http://localhost:8000/?lang=en&cheats=1` — English + cheats

## Project Structure

```
├── server.go           # Go HTTP server
├── dist/               # Game client files (embedded in Go binary)
│   ├── index.html      # Main page
│   ├── game.js         # Game loader
│   ├── streaming_files.txt  # List of streaming assets
│   └── modules/        # WASM modules
└── vcsky/              # Cached assets (created automatically)
    └── fetched/        # Streaming assets from CDN
        ├── models/
        │   └── gta3.img/   # 4310 model/texture files
        └── audio/
            └── sfx.raw/    # 0.mp3 - 9940.mp3 sound effects
```

## Features

- Gamepad emulation for touch devices
- Cloud saves via js-dos key
- English/Russian language support
- Built-in cheat engine (memory scanner, cheats)
- Mobile touch controls

## Controls (Touch)

Touch controls appear automatically on mobile devices. Virtual joysticks for movement and camera, context-sensitive action buttons.

## Cheats

Enable with `?cheats=1`, press **F3** to open menu:
- Memory scanner (find/edit values)
- All classic GTA VC cheats
- AirBreak (noclip mode)

## Offline Setup

For fully offline play without CDN dependency, download all assets using the `-download` flag:

```bash
# Build and download all assets
go run server.go -download
```

This downloads:
- ~4310 model/texture files (~150MB)
- ~9940 sound effect files (~50MB)

Assets are cached in `vcsky/fetched/` and served locally on subsequent requests.

## License

MIT.

---

**Authors:** DOS Zone ([@specialist003](https://github.com/okhmanyuk-ev), [@caiiiycuk](https://www.youtube.com/caiiiycuk), [@SerGen](https://t.me/ser_var))

**Deobfuscated by**: [@Lolendor](https://github.com/Lolendor)