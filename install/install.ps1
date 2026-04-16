param(
  [string]$Repo = $env:MATRIX_REPO,
  [string]$Version = $env:MATRIX_VERSION,
  [string]$MatrixHome = $env:MATRIX_HOME
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Repo)) {
  $Repo = "Josepavese/matrix"
}
if ([string]::IsNullOrWhiteSpace($Version)) {
  $Version = "latest"
}
if ([string]::IsNullOrWhiteSpace($MatrixHome)) {
  $MatrixHome = Join-Path $env:LOCALAPPDATA "Matrix"
}

$arch = switch ($env:PROCESSOR_ARCHITECTURE.ToLowerInvariant()) {
  "amd64" { "amd64" }
  "x86_64" { "amd64" }
  "arm64" { "arm64" }
  default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

$api = "https://api.github.com/repos/$Repo/releases/latest"
if ($Version -ne "latest") {
  $api = "https://api.github.com/repos/$Repo/releases/tags/$Version"
}

$release = Invoke-RestMethod -Uri $api -Headers @{ "User-Agent" = "matrix-installer" }
$asset = $release.assets | Where-Object {
  $_.name -like "*_windows_$arch.zip"
} | Select-Object -First 1

if ($null -eq $asset) {
  throw "No Matrix release asset found for windows_$arch in $Repo $Version"
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("matrix-install-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

try {
  $archive = Join-Path $tmp "matrix.zip"
  Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $archive
  Expand-Archive -Path $archive -DestinationPath $tmp -Force

  foreach ($dir in @("bin", "configs", "data", "logs", "artifacts", "backups", "tmp")) {
    New-Item -ItemType Directory -Force -Path (Join-Path $MatrixHome $dir) | Out-Null
  }

  Copy-Item -Path (Join-Path $tmp "matrix.exe") -Destination (Join-Path $MatrixHome "bin\matrix.exe") -Force

  $srcConfigs = Join-Path $tmp "configs"
  if (Test-Path $srcConfigs) {
    Get-ChildItem -Path $srcConfigs -Recurse -File | ForEach-Object {
      $rel = $_.FullName.Substring($srcConfigs.Length).TrimStart("\", "/")
      $dest = Join-Path (Join-Path $MatrixHome "configs") $rel
      New-Item -ItemType Directory -Force -Path (Split-Path $dest -Parent) | Out-Null
      if (-not (Test-Path $dest)) {
        Copy-Item -Path $_.FullName -Destination $dest
      }
    }
  }
}
finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

Write-Host "Matrix installed."
Write-Host "PAL home: $MatrixHome"
Write-Host "Binary:   $(Join-Path $MatrixHome 'bin\matrix.exe')"
Write-Host ""
Write-Host "Run:"
Write-Host "  `$env:MATRIX_HOME = '$MatrixHome'"
Write-Host "  & '$MatrixHome\bin\matrix.exe' home"
Write-Host "  & '$MatrixHome\bin\matrix.exe' bootstrap doctor"
