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

At startup, the binary resolves the PAL home and changes into it before loading configs or the vault.
`MATRIX_HOME` is an advanced override for development, smoke tests, staging, and parallel installs. Normal usage does not require setting it.

The installer prepares a user-level `matrix` command. After installation, the normal UX is:

```bash
matrix home
matrix bootstrap doctor
matrix run
```

## Linux And macOS

Install latest release:

```bash
curl -fsSL https://github.com/Josepavese/matrix/releases/latest/download/install.sh | sh
```

Install a specific release:

```bash
MATRIX_VERSION=v0.1.17
curl -fsSL "https://github.com/Josepavese/matrix/releases/download/${MATRIX_VERSION}/install.sh" | env MATRIX_VERSION="$MATRIX_VERSION" sh
```

Custom PAL home:

```bash
curl -fsSL https://github.com/Josepavese/matrix/releases/latest/download/install.sh | env MATRIX_HOME="$HOME/.matrix-pal" sh
```

Run:

```bash
matrix home
matrix bootstrap doctor
matrix run
```

If the current shell cannot find `matrix`, open a new shell. The installer adds `~/.local/bin` to the shell profile when needed.

## Windows PowerShell

Install latest release:

```powershell
irm https://github.com/Josepavese/matrix/releases/latest/download/install.ps1 | iex
```

Install a specific release:

```powershell
$env:MATRIX_VERSION = "v0.1.17"
irm "https://github.com/Josepavese/matrix/releases/download/$env:MATRIX_VERSION/install.ps1" | iex
```

Custom PAL home:

```powershell
$env:MATRIX_HOME = "$env:USERPROFILE\.matrix-pal"
irm https://github.com/Josepavese/matrix/releases/latest/download/install.ps1 | iex
```

Run:

```powershell
matrix home
matrix bootstrap doctor
matrix run
```

If the current shell cannot find `matrix`, open a new PowerShell session. The installer adds the Matrix `bin` directory to the user `Path`.

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
