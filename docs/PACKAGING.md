# Packaging

Goal: a user downloads one file and runs it. No installer. The Windows build
has no runtime dependencies; the Linux build uses the standard desktop
libraries listed below. Both embed the GUI (Gio), read side, write engine,
all 5 data JSONs, and all 2752 item icons via `go:embed`.

## Build

`bash app/build.sh` (any OS, incl. WSL) produces:
- `app/dist/ERMerchantEditor-windows-amd64.exe` — windows/amd64 GUI, **87MB** single
  file (`-H windowsgui -s -w -trimpath`, CGO_ENABLED=0 — Gio renders via
  Direct3D syscalls on Windows, no cgo, so this cross-compiles from
  anywhere). No launch-time extraction (the PyInstaller-era exe re-unpacked
  85MB of icons to temp on every start).
- `app/dist/ERMerchantEditor-linux-amd64` — linux/amd64 GUI, one executable
  with the same embedded data. It is built natively with cgo for Gio's Linux
  backend and is intended for x86_64 Linux desktops.
- `app/dist/install-linux-desktop.sh`,
  `app/dist/io.github.daniele.ERMerchantEditor.desktop`, and
  `app/dist/io.github.daniele.ERMerchantEditor.png` — optional per-user
  desktop integration. Linux associates a window icon through these desktop
  resources rather than through the executable itself.
- `app/dist/shopwrite/<goos>-<goarch>/shopwrite[.exe]` — the CLI, 2
  targets, standalone (schema embedded, no data/ sibling).

Windows exe metadata (icon/manifest/version) comes from the **committed**
`app/cmd/editor/rsrc_windows_amd64.syso`; regenerate only after editing
`app/winres/*` with
`go tool go-winres make --in app/winres/winres.json --out app/cmd/editor/rsrc --arch amd64`.
Icon group uses resource ordinal **#1** — that's where Gio's Windows
backend looks up the titlebar/window icon. `app/winres/icon.png` is the
256px entry extracted from `app/icon.ico` (go-winres rejects
PNG-compressed ICO entries; a plain PNG input works and generates all
sizes).

Toolchain: apt go1.22 + `GOTOOLCHAIN=auto` fetches the pinned ≥1.25
toolchain once (go.mod directive). First build needs network for the
module cache (gio/x/zenity/compress); offline after that.

## Linux GUI (release and development)

The Windows target is cgo-free. Building the GUI natively on Linux needs cgo
and these dev packages:
`libwayland-dev libx11-dev libx11-xcb-dev libxkbcommon-x11-dev
libgles2-mesa-dev libegl1-mesa-dev libxcursor-dev libxfixes-dev
libvulkan-dev`. The release executable needs the corresponding runtime
libraries provided by a normal Linux desktop, plus the `zenity` binary for
file dialogs. Run it with:

```
chmod +x ERMerchantEditor-linux-amd64
./ERMerchantEditor-linux-amd64

# Optional: install a launcher and taskbar icon for this user.
./install-linux-desktop.sh
```

WSLg runs the window fine (Mesa EGL warnings at startup are normal). It can
report a 1.0 UI scale even when Windows uses 125%; the native Linux desktop
scale remains automatic. For a WSLg-only override, launch with
`ER_MERCHANT_EDITOR_SCALE=1.25 ./ERMerchantEditor-linux-amd64` (valid range:
0.75 through 2.0).

## CI (`.github/workflows/release.yml`)

`ubuntu-latest`, plain `setup-go` (stable) + Linux GUI build dependencies →
`go vet` + `go test ./...` (fixture-dependent tests self-skip; the gitignored
save fixture never reaches CI) → `bash app/build.sh` → on `v*` tag push,
attach the Windows and Linux GUIs to a **draft** GitHub Release; on
`workflow_dispatch`, upload both as a workflow artifact instead. No Windows
runner, no Python, no secrets beyond `GITHUB_TOKEN`.

## Superseded

- **Tauri era** (v0.1.0, shipped then discarded 2026-07-27): Tauri v2 +
  React + FastAPI backend frozen two ways + 3-OS CI with 7 installer
  formats. Worked, but 4 toolchains and 2 IPC boundaries for a local
  file-transform tool. In git history before the pivot commit.
- **Dear PyGui era** (same day, 2026-07-27): single Python process +
  PyInstaller onefile (103MB exe, 85MB temp re-extract each launch),
  `shopwrite` as a subprocess. Fully working and user-verified; replaced
  hours later by this Go/Gio rewrite for a one-language codebase, a truly
  static exe, and `go build`-only packaging. The PyInstaller/venv/WSL
  gotchas that era documented (hybrid venvs on shared checkouts, --windowed
  stdout traps, _MEIPASS resolution) are in git history if ever needed.
