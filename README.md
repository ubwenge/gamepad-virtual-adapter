# Gamepad Virtual Adapter

`gamepad-virtual-adapter` reads input reports from a USB game controller and turns configured
signals into macOS keyboard events. It includes a sample mapping for a PDP
controller (`0E6F:0401`), with the left stick mapped to `WASD` and face buttons
mapped to common game keys.

## Requirements

- macOS (keyboard synthesis uses ApplicationServices)
- Go 1.27 or newer
- libusb, for example: `brew install libusb`
- Accessibility/Input Monitoring permission for the terminal or built binary
  that runs the mapper

## Build and run

Build the application from the repository root:

```sh
go build -o gamepad-virtual-adapter .
```

Run the included PDP mapping:

```sh
./gamepad-virtual-adapter
```

The program searches for the configured USB vendor and product IDs, then sends
keyboard down/up events while the corresponding controller signals are active.
Press `Ctrl-C` to stop it; held output keys are released on exit.

Run the tests with:

```sh
go test ./...
```

## Install with Homebrew

The released macOS binaries are available for Apple Silicon and Intel Macs:

```sh
brew install --cask ubwenge/tap/gamepad-virtual-adapter
```

The Cask installs `libusb` automatically. Grant the installed binary
Accessibility and Input Monitoring access before running it. MVP releases are
not yet signed or notarized; if macOS blocks the binary, remove its quarantine
attribute:

```sh
xattr -dr com.apple.quarantine "$(which gamepad-virtual-adapter)"
```

You can also download the matching archive and its checksum from the project's
[GitHub Releases](https://github.com/ubwenge/gamepad-virtual-adapter/releases).

## Publishing a release

The release workflow runs when a semantic version tag such as `v0.1.0` is
pushed. It creates macOS arm64 and amd64 archives, uploads them to GitHub
Releases, and updates the `ubwenge/homebrew-tap` Cask.

Before the first release, create the public `ubwenge/homebrew-tap` repository
and add a `HOMEBREW_TAP_GITHUB_TOKEN` repository secret here. The secret must
be a fine-grained GitHub token restricted to that tap repository with Contents
read/write permission.

```sh
git tag v0.1.0
git push origin v0.1.0
```

## Configure another controller

1. Create a local configuration from the template:

   ```sh
   cp config.toml.template config.local.toml
   ```

2. Add your controller IDs, then record its reports. Monitor mode writes raw
   reports to standard output, making it easy to save them:

   ```sh
   ./gamepad-virtual-adapter --monitor --config config.local.toml > controller-capture.txt
   ```

3. Press one controller input at a time and use the captured report for each
   signal. In `config.local.toml`, put exactly one bracketed byte span in every
   configured report. A one-byte span is treated as a bit mask; a multi-byte
   span is an exact match.

4. Add one keyboard binding for every configured signal, then run:

   ```sh
   ./gamepad-virtual-adapter --config config.local.toml
   ```

Mapper mode requires valid IDs, at least one signal, and exactly one unique key
binding for each configured signal. The template lists supported signal groups:
face buttons, D-pad, system buttons, shoulders, thumb clicks, and stick
directions. Directional rules for the same thumb stick are mutually exclusive;
D-pad directions can be combined for diagonals.

Supported output key names are `A`–`Z`, `0`–`9`, `Space`, `Escape`, `Enter`,
`Tab`, `Backspace`, `ArrowUp`, `ArrowDown`, `ArrowLeft`, `ArrowRight`,
`LeftShift`, `LeftControl`, `LeftOption`, and `LeftCommand`.

## Diagnostics

Use `--debug` to log each USB configuration, interface, and endpoint attempted
during device setup:

```sh
./gamepad-virtual-adapter --config config.local.toml --debug
```

This is useful when the controller is detected but its USB interface cannot be
claimed. If synthesized keys do not reach other apps, grant the running
application Accessibility/Input Monitoring access in macOS privacy settings.
