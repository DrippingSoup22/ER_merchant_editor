# Packaging and releases

The desktop GUI shares one Go application core, but each GUI is built and
tested natively. Windows is the only cgo-free GUI target; Linux and macOS use
their native window and graphics toolchains.

## Output layout

Build output is ignored under `dist/`:

- `scripts/build-windows.sh` produces
  `dist/windows-amd64/ERMerchantEditor.exe` and `shopwrite.exe`.
- `scripts/build-linux.sh` produces
  `dist/ERMerchantEditor-linux-<arch>.tar.gz`.
- `scripts/build-macos.sh` produces a real `.app` bundle and
  `dist/ERMerchantEditor-macos-<arch>.zip`.
- `scripts/build.sh` is a developer convenience dispatcher for the current
  host. Release CI calls each native target script directly.

All runtime catalog data and item icons are embedded. The CLI imports only
`internal/assets/data`, so it does not carry the large icon collection.

## Windows

Windows resources are committed at
`cmd/ermerchanteditor/rsrc_windows_amd64.syso`. Source metadata and artwork
live in `packaging/windows/`.

Regenerate the resource object after changing that metadata:

```sh
go tool go-winres make \
  --in packaging/windows/winres/winres.json \
  --out cmd/ermerchanteditor/rsrc \
  --arch amd64
```

The Windows target uses `CGO_ENABLED=0` and can also be cross-built from Linux
or macOS.

## Linux

Linux builds require gcc, pkg-config, Wayland, X11, xkbcommon, EGL/GLES,
libffi, Xcursor, Xfixes, and Vulkan development packages. CI builds against
Ubuntu 22.04 to avoid unnecessarily raising the glibc baseline.

The current dialog adapter requires `zenity`, `matedialog`, or `qarma` at
runtime. This dependency is explicit in the archive README. The platform
interface in `internal/platform/dialogs` allows a later XDG-portal adapter
without touching the editor core or views.

The initial artifact is a tarball with the binary and desktop metadata.
AppImage or Flatpak can be added under `packaging/linux` without changing the
Go package layout.

## macOS

macOS builds run natively with Xcode and produce a standard `.app` containing
`Info.plist`, an `.icns` icon, and the GUI executable. CI builds separate Intel
and Apple Silicon archives.

Local and CI builds are ad-hoc signed when `MACOS_SIGN_IDENTITY` is unset. A
public macOS release should provide a Developer ID identity, notarize the zip,
and staple the result before publication. Those credentials are deliberately
not assumed by the source build.

## CI

`.github/workflows/release.yml` contains native jobs for:

- Ubuntu 22.04 x86-64
- Windows Server 2025 x86-64
- macOS 15 Intel
- macOS 15 Apple Silicon

Every native job runs `go test ./...` and `go vet ./...` before packaging. A
final job combines their artifacts and either uploads a manual workflow
artifact or attaches them to a draft tag release.
