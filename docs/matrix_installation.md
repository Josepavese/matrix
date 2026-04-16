# Matrix Installation

Matrix installs into one PAL home. The installer does not clone the repository and does not scatter runtime files across the current directory.

## PAL Home

Resolution order:

1. `MATRIX_HOME`
2. OS default

Defaults:

| OS | Default PAL home |
| --- | --- |
| Linux | `$XDG_DATA_HOME/matrix` or `$HOME/.local/share/matrix` |
| macOS | `$HOME/Library/Application Support/Matrix` |
| Windows | `%LOCALAPPDATA%\Matrix` |

Layout:

```text
MATRIX_HOME/
  bin/          matrix executable
  configs/      seed configs copied from the release archive
  data/         matrix-vault.db and durable local state
  logs/         runtime logs
  artifacts/    local generated artifacts
  backups/      vault backups
  tmp/          temporary runtime workspace
```

The binary changes into `MATRIX_HOME` at startup unless it detects repository development mode.

## Linux And macOS

Install latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.sh | sh
```

Install a specific release:

```bash
MATRIX_VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.sh | sh
```

Custom PAL home:

```bash
MATRIX_HOME="$HOME/.matrix-pal" curl -fsSL https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.sh | sh
```

Run:

```bash
export MATRIX_HOME="${XDG_DATA_HOME:-$HOME/.local/share}/matrix"
export PATH="$MATRIX_HOME/bin:$PATH"

matrix home
matrix bootstrap doctor
matrix run
```

On macOS, use:

```bash
export MATRIX_HOME="$HOME/Library/Application Support/Matrix"
export PATH="$MATRIX_HOME/bin:$PATH"
```

## Windows PowerShell

Install latest release:

```powershell
irm https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.ps1 | iex
```

Install a specific release:

```powershell
$env:MATRIX_VERSION = "v0.1.0"
irm https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.ps1 | iex
```

Custom PAL home:

```powershell
$env:MATRIX_HOME = "$env:USERPROFILE\.matrix-pal"
irm https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.ps1 | iex
```

Run:

```powershell
$env:MATRIX_HOME = "$env:LOCALAPPDATA\Matrix"
& "$env:MATRIX_HOME\bin\matrix.exe" home
& "$env:MATRIX_HOME\bin\matrix.exe" bootstrap doctor
& "$env:MATRIX_HOME\bin\matrix.exe" run
```

## Release Artifacts

GitHub Actions and GoReleaser produce:

- `linux_amd64`
- `linux_arm64`
- `darwin_amd64`
- `darwin_arm64`
- `windows_amd64`
- `windows_arm64`

Archives include:

- `matrix` or `matrix.exe`
- `configs/`
- installer scripts
- installation and timeout/recovery docs

## No Clone Rule

End users should not clone the repository for installation. Clone is only for development.

Installer behavior:

- downloads only the matching release archive;
- extracts into a temporary directory;
- copies the executable into `MATRIX_HOME/bin`;
- copies missing seed configs into `MATRIX_HOME/configs`;
- creates required runtime directories;
- removes temporary files.

## Uninstall

Remove the PAL home:

Linux:

```bash
rm -rf "${XDG_DATA_HOME:-$HOME/.local/share}/matrix"
```

macOS:

```bash
rm -rf "$HOME/Library/Application Support/Matrix"
```

Windows PowerShell:

```powershell
Remove-Item -Recurse -Force "$env:LOCALAPPDATA\Matrix"
```
