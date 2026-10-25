<p align="center">
  <img src="assets/brand/logo.png" alt="DaePlex" width="440">
</p>

# DaePlex Text Editor. IT WORKS!

> Formerly `pieces-store`, renamed 2026-10-20 — see `CLAUDE.md`'s Current
> phase for the story. This README predates several architecture renames
> (`piecestore` → `piecetable`, the `keyengine`/`offset`/`viewmanager`
> split) and isn't the source of truth for current structure — see
> `CLAUDE.md`.

A Go-based text editor architecture utilizing a **Piece Table** (Piece Store) data structure. The project is designed with a clean separation of concerns and features three primary interfaces:
1. **GUI**: A cross-platform graphical user interface built with [Gio UI](https://gioui.org).
2. **TUI**: A terminal-based user interface built with [tview](https://github.com/rivo/tview).
3. **API (Dev)**: The core engine runner and backend testing environment.

Development is fully containerized and supports **live-rebuilding (hot-reloading)** using [Air](https://github.com/air-verse/air) across all services.

---

## Project Structure

```text
├── cmd/
│   ├── api/          # Entry point for the dev engine
│   ├── gui/          # Entry point for the GUI app
│   └── tui/          # Entry point for the TUI app
├── internal/
│   ├── gui-base/     # Gio UI window setup and widgets
│   ├── tui-base/     # tview application initialization
│   └── piecestore/   # Core Piece Table buffer implementation (insert/delete/history)
├── docker-compose.yml# Multi-service container specification
├── Taskfile.yaml     # Task automation commands
└── .env              # Host environment mappings (automatically generated)
```

---

## Getting Started

### Prerequisites

Ensure you have the following installed on your host system:
* [Go](https://go.dev/doc/install) (1.26+)
* [Docker](https://docs.docker.com/) & [Docker Compose](https://docs.docker.com/compose/)
* [Task](https://taskfile.dev/) (optional, for running automation tasks)

---

## Development Workflow & Commands

We use a `Taskfile.yaml` to automate development tasks. If you do not have `task` installed, you can run the raw commands listed inside the `Taskfile.yaml`.

### Common Go Commands
* **Format code**:
  ```bash
  task fmt
  ```
* **Run static analysis**:
  ```bash
  task vet
  ```
* **Run unit tests**:
  ```bash
  task test
  ```
* **Build binaries locally**:
  ```bash
  task build
  ```
* **Clean temporary files**:
  ```bash
  task clean
  ```

### Running the Services

#### 1. Running the GUI Service (Docker)
The GUI container shares your host X11 and Wayland sockets, allowing hardware acceleration (`/dev/dri`) and GPU drawing directly to your desktop:
```bash
task gui
```

#### 2. Running the TUI Service (Interactive)
Terminal interfaces need interactive TTY access. Choose one of the following:
* **Directly on host (Recommended)**:
  ```bash
  task tui
  ```
* **Inside Docker container**:
  ```bash
  task tui-docker
  ```

#### 3. Running the Engine API Service (Docker)
Runs the test suite/engine logs inside Docker under hot-reload:
```bash
task dev
```

#### 4. Managing All Services
* **Start all services**:
  ```bash
  task up
  ```
* **Stop all services**:
  ```bash
  task down
  ```

---

## Troubleshooting & Fix Reference

Below is a record of fixes implemented to resolve issues during initial setup:

### 1. Air Config Directory Collision (Exit Code 126 / Permission Denied)
* **Issue**: Air configurations (`.air.*.toml`) pointed `tmp_dir` and the executable output `bin` to the same folder path (e.g. `tmp/gui`). Air created the folder `tmp/gui`, compiled the binary into it (as `tmp/gui/main`), and then tried to execute the directory path `tmp/gui` directly. This resulted in a Linux `Permission Denied` (Exit Code 126) error.
* **Fix**: Separated build directories and binary names in Air configurations (e.g. `tmp_dir = "tmp/gui-build"` and `bin = "./tmp/gui-app"`).

### 2. Missing Container Fonts (Blank White GUI Screen)
* **Issue**: Docker containers based on minimal Linux distributions do not bundle font servers (`fontconfig`) or font files. Gio applications relying on system fonts displayed a blank white screen because text glyphs failed to render.
* **Fix**:
  1. Updated `internal/gui-base/base.go` to explicitly bundle the `gofont` Go package and assign it to the theme's shaper (`theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))`). This embeds font files inside the compiled executable.
  2. Updated the `Dockerfile` to install system fonts and `fontconfig` as fallbacks.

### 3. Missing `ops.Reset()` in Frame Event Loop
* **Issue**: Gio's immediate-mode GUI uses an `op.Ops` struct to record GPU draw commands. The operations were not being cleared at the start of the frame, causing drawing command logs to grow infinitely and corrupting the render buffer.
* **Fix**: Added `ops.Reset()` at the start of the `app.FrameEvent` case.

### 4. Docker File Owner Permissions (Root-Owned Files)
* **Issue**: Running Docker Compose without explicit host environment mappings caused the container to fall back to `root:root`. Temporary build files created in mounted host directories became unmodifiable without `sudo`.
* **Fix**: Added a `.env` file mapping `UID` and `GID` dynamically, and updated `docker-compose.yml` to run container processes under the host user identity (`user: "${UID:-1000}:${GID:-1000}"`).

### 5. Piece Table Edge Case Panics
* **Issue**: Appending at the absolute end of the buffer caused out-of-range slice panics, and deleting sections within a single piece discarded the remaining right-hand text slice due to incorrect `else-if` conditional checks.
* **Fix**: Refactored insertion and deletion logic in `internal/piecestore/store.go` and verified behavior with robust `go test` unit coverage.
