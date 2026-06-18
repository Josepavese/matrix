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
$checksumAsset = $release.assets | Where-Object {
  $_.name -eq "checksums.txt"
} | Select-Object -First 1

if ($null -eq $asset) {
  throw "No Matrix release asset found for windows_$arch in $Repo $Version"
}

function Test-MatrixChecksum {
  param(
    [string]$ChecksumFile,
    [string]$ArchivePath,
    [string]$AssetName
  )

  $expected = $null
  foreach ($line in Get-Content -Path $ChecksumFile) {
    $parts = $line.Trim() -split "\s+"
    if ($parts.Count -ge 2 -and $parts[1].TrimStart([char]"*") -eq $AssetName) {
      $expected = $parts[0]
      break
    }
  }
  if ([string]::IsNullOrWhiteSpace($expected)) {
    throw "checksums.txt is available but has no entry for $AssetName"
  }

  $actual = (Get-FileHash -Algorithm SHA256 -Path $ArchivePath).Hash.ToLowerInvariant()
  if ($actual -ne $expected.ToLowerInvariant()) {
    throw "checksum verification failed for $AssetName"
  }

  Write-Host "Verified checksum for $AssetName"
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("matrix-install-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

try {
  $archive = Join-Path $tmp "matrix.zip"
  Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $archive
  if ($null -ne $checksumAsset) {
    $checksumFile = Join-Path $tmp "checksums.txt"
    Invoke-WebRequest -Uri $checksumAsset.browser_download_url -OutFile $checksumFile
    Test-MatrixChecksum -ChecksumFile $checksumFile -ArchivePath $archive -AssetName $asset.name
  }
  else {
    Write-Host "No checksums.txt release asset found; skipping checksum verification."
  }
  Expand-Archive -Path $archive -DestinationPath $tmp -Force
  $extractedBinary = Join-Path $tmp "matrix.exe"
  if (-not (Test-Path $extractedBinary)) {
    throw "release asset $($asset.name) does not contain matrix.exe at archive root"
  }

  foreach ($dir in @("bin", "configs", "data", "logs", "artifacts", "backups", "tmp")) {
    New-Item -ItemType Directory -Force -Path (Join-Path $MatrixHome $dir) | Out-Null
  }

  $binDir = Join-Path $MatrixHome "bin"
  $binary = Join-Path $binDir "matrix.exe"
  Copy-Item -Path $extractedBinary -Destination $binary -Force

  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if ([string]::IsNullOrWhiteSpace($userPath)) {
    $userPath = ""
  }
  $pathParts = $userPath -split ";" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
  $hasBin = $false
  foreach ($part in $pathParts) {
    if ($part.TrimEnd("\") -ieq $binDir.TrimEnd("\")) {
      $hasBin = $true
      break
    }
  }
  if (-not $hasBin) {
    $newUserPath = if ($userPath.Trim().Length -gt 0) { "$userPath;$binDir" } else { $binDir }
    [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
  }
  if (($env:Path -split ";") -notcontains $binDir) {
    $env:Path = "$env:Path;$binDir"
  }

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
Write-Host "  matrix home"
Write-Host "  matrix bootstrap doctor"
Write-Host ""
Write-Host "If this shell cannot find matrix, open a new PowerShell session."
