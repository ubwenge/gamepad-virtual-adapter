# Contributor Guide

## Project

`gamepad-virtual-adapter` is a macOS-only Go command-line application. It reads
USB controller input through `libusb` and emits macOS keyboard events through
ApplicationServices.

## Development

- Use the Go version declared in `go.mod`.
- Install native dependencies on macOS with `brew install libusb pkg-config`.
- Run `go test ./...`, `go vet ./...`, and ensure `gofmt -l .` produces no
  output before submitting changes.
- Keep monitor-mode output on standard output; diagnostic and status messages
  belong on standard error.
- Preserve the separation between monitor mode and keyboard synthesis so monitor
  mode never requests keyboard-related macOS permissions.

## Configuration and permissions

- `config.toml.template` is a user-facing template; keep it valid TOML and do
  not add local controller captures or machine-specific paths.
- The application needs Accessibility and Input Monitoring permission when it
  synthesizes keystrokes. Document any new permission requirements in the
  README.

## Releases

- CI runs native macOS checks in `.github/workflows/ci.yml`.
- Push a semantic version tag (`vX.Y.Z`) to publish macOS arm64 and amd64
  archives, checksums, and the Homebrew Cask.
- Release publishing requires the `HOMEBREW_TAP_GITHUB_TOKEN` secret, scoped to
  `ubwenge/homebrew-tap` with Contents read/write permission.
- MVP release binaries are intentionally unsigned and not notarized; do not add
  signing or notarization requirements without updating the release workflow
  and installation documentation together.
