# 🎮 GTA Vice City — HTML5 Browser Port

Play GTA: Vice City directly in your browser via WebAssembly! 🌴

---

## ✨ Features

- 🌐 Runs entirely in your browser (WebAssembly)
- 💾 No installation required
- 🚀 Easy setup
- 🎮 Gamepad support + touch emulation
- 📱 Mobile-friendly controls
- 🔧 Built-in cheats & memory scanner
- 📴 Offline play support
- 🌍 Multi-language support
- ⚡ CDN-backed asset streaming with local caching

---

## ⚡ Quick Start

1. Download the latest release from the [Releases page](https://github.com/adambenhassen/reVCDOS/releases)
2. Extract and run the executable
3. Open 👉 **http://localhost:8000**

---

## 🐳 Docker (Recommended)

```bash
docker compose up -d --build
```

### 🔧 Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8000` | Server port |
| `AUTH_LOGIN` | — | Username for Basic Auth |
| `AUTH_PASSWORD` | — | Password for Basic Auth |
| `CDN` | `https://cdn.dos.zone/vcsky/` | Asset CDN URL |
| `DOWNLOAD_CACHE` | `false` | Pre-download assets on startup (`1` or `true`) |
| `WORKERS` | `8` | Parallel download threads |

**Example with auth:**
```bash
AUTH_LOGIN=admin AUTH_PASSWORD=secret docker compose up -d --build
```

---

## 🖥️ Go Server

```bash
go run server.go
```

### 🎛️ Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8000` | Server port |
| `-login` | — | Basic Auth username |
| `-password` | — | Basic Auth password |
| `-cdn` | `https://cdn.dos.zone/vcsky/` | Asset CDN URL |
| `-download` | — | Download all assets and exit |
| `-download-cache` | — | Download assets in background while serving |
| `-workers` | `8` | Parallel download threads |

**Examples:**
```bash
go run server.go -port 3000                    # Custom port
go run server.go -login admin -password 123    # Enable auth
go run server.go -download                     # Offline mode setup
go run server.go -download-cache               # Background download
```

> 💡 **Tip:** Build a single binary with `go build -o server server.go`

---

## 🔗 URL Parameters

Customize gameplay by adding these to the URL:

| Parameter | Values | Description |
|-----------|--------|-------------|
| `lang` | `en` | Game language |
| `cheats` | `1` | Enable cheat menu (F3) |
| `fullscreen` | `0` | Disable auto-fullscreen |
| `max_fps` | `1-240` | Limit frame rate |
| `request_original_game` | `1` | Prompt for original game files |

**Example:** `http://localhost:8000/?lang=en&cheats=1`

---

## 🕹️ Cheats

Add `?cheats=1` to URL, then press **F3** to open the cheat menu:

- 🔍 Memory scanner (find & edit values)
- 🎯 All classic GTA VC cheats
- 👻 AirBreak (noclip mode)

---

## 📱 Mobile Controls

Touch controls appear automatically on mobile devices:
- Virtual joysticks for movement & camera
- Context-sensitive action buttons

---

## 📴 Offline Setup

Download all assets for fully offline play:

```bash
go run server.go -download
```

This fetches:
- 📦 ~4,310 model/texture files (~150MB)
- 🔊 ~9,940 sound effects (~50MB)

Assets are cached in `vcsky/fetched/` for future use.

---

## 📁 Project Structure

```
├── server.go              # 🖥️ Go HTTP server
├── dist/                  # 🎮 Game client files
│   ├── index.html         # Main page
│   ├── game.js            # Game loader
│   ├── streaming_files.txt
│   └── modules/           # WASM modules
└── vcsky/                 # 📦 Cached assets (auto-created)
    └── fetched/
        ├── models/gta3.img/
        └── audio/sfx.raw/
```

---

## 📄 License

MIT

---

## 👥 Credits

**Authors:** DOS Zone ([@specialist003](https://github.com/okhmanyuk-ev), [@caiiiycuk](https://www.youtube.com/caiiiycuk), [@SerGen](https://t.me/ser_var))

**Contributors:** [@Lolendor](https://github.com/Lolendor)
