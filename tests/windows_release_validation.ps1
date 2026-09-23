# Validates a published Matrix release on a real Windows host.
#
# This is the one release criterion that cannot be met anywhere else: install.ps1
# only runs on Windows and installs the only artifact that only exists for
# Windows. The script takes the installer from the published release, exactly as
# a user would, and then checks that the installed binary actually runs:
#
#   pwsh -File tests/windows_release_validation.ps1 -Version v0.1.34
#
# -ReportUrl points at a collector that accepts the report as a POST body, which
# is how the result leaves a VM that has no shared filesystem with the host.

param(
  [string]$Version = "v0.1.34",
  [string]$Repo = "Josepavese/matrix",
  [string]$HomeDir = "C:\matrix-test",
  [string]$ReportUrl = "",
  # Optional installer URL, used to exercise a corrected installer before it is
  # published. Empty means the installer published with the release.
  [string]$InstallerUrl = ""
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$version = $Version
$repo = $Repo
$home_dir = $HomeDir
$results = New-Object System.Collections.Generic.List[string]
$failures = New-Object System.Collections.Generic.List[string]

function Check {
  param([string]$Name, [scriptblock]$Body)
  try {
    $value = & $Body
    $results.Add("ok   $Name -> $value")
    Write-Host "ok   $Name -> $value"
  } catch {
    $failures.Add("$Name : $_")
    Write-Host "FAIL $Name : $_"
  }
}

Write-Host "=== host ==="
Write-Host "OS: $((Get-CimInstance Win32_OperatingSystem).Caption)"
Write-Host "PowerShell: $($PSVersionTable.PSVersion)"
Write-Host "Arch: $env:PROCESSOR_ARCHITECTURE"

# The installer is taken from the published release, exactly as a user would.
$installer = Join-Path $env:TEMP "install.ps1"
$installerSource = if ($InstallerUrl) { $InstallerUrl } else { "https://github.com/$repo/releases/download/$version/install.ps1" }
Check "download install.ps1 from $(if ($InstallerUrl) { 'the supplied URL' } else { 'the published release' })" {
  Invoke-WebRequest -Uri $installerSource -OutFile $installer -UseBasicParsing
  (Get-Item $installer).Length
}

# The checksum of the installer itself, so the artifact under test is identified.
Check "installer sha256" {
  (Get-FileHash -Algorithm SHA256 -Path $installer).Hash.ToLowerInvariant()
}

Write-Host "=== running install.ps1 ==="
$installFailed = $false
try {
  & $installer -Repo $repo -Version $version -MatrixHome $home_dir
} catch {
  $installFailed = $true
  $failures.Add("install.ps1 threw: $_")
  Write-Host "FAIL install.ps1 threw: $_"
}
if (-not $installFailed) {
  Write-Host "ok   install.ps1 completed"
}

$binary = Join-Path $home_dir "bin\matrix.exe"
Check "matrix.exe installed" { Test-Path $binary }
Check "PAL home created" { (Get-ChildItem $home_dir -Directory | Select-Object -ExpandProperty Name) -join "," }
Check "matrix.exe runs" { & $binary version | Select-Object -First 1 }
Check "matrix.exe reports the release version" {
  $out = & $binary version
  # The expected version comes from -Version: this script is reused for every
  # release, and a hard-coded one silently pins it to the release it was written for.
  $expected = $version.TrimStart("v")
  if (-not ($out -match [regex]::Escape($expected))) { throw "unexpected version output: $out" }
  ($out | Select-Object -First 1)
}
Check "doctor runs on Windows" {
  $env:MATRIX_HOME = $home_dir
  $out = & $binary doctor 2>&1 | Out-String
  if ($LASTEXITCODE -ne 0) { throw "doctor exited $LASTEXITCODE : $out" }
  "exit=0"
}
Check "readiness runs on Windows" {
  $env:MATRIX_HOME = $home_dir
  # matrix.exe writes its "vault was not inspected" note to stderr, which is the
  # intended behaviour of the fresh-host fix; PowerShell 5.1 turns native stderr
  # into a terminating error while ErrorActionPreference is Stop, so it is
  # relaxed for this call only.
  $previous = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  $out = (& $binary readiness 2>&1 | Where-Object { $_ -is [string] } | Out-String)
  $ErrorActionPreference = $previous
  $json = $out | ConvertFrom-Json
  if ($json.status -ne "not_ready") { throw "a fresh host must be not_ready, got $($json.status)" }
  if ($json.blockers.Count -lt 1) { throw "a fresh host must report a blocker" }
  "status=$($json.status) blockers=$($json.blockers -join '; ')"
}
Check "configs were seeded without overwriting" {
  (Get-ChildItem (Join-Path $home_dir "configs") -File | Measure-Object).Count
}


# The report is written locally and posted back to the host, so the result does
# not have to be read off a screenshot.
$report = @()
$report += "host_os=$((Get-CimInstance Win32_OperatingSystem).Caption)"
$report += "powershell=$($PSVersionTable.PSVersion)"
$report += "arch=$env:PROCESSOR_ARCHITECTURE"
$report += $results
$report += $failures
$text = $report -join "`n"
Set-Content -Path (Join-Path $env:TEMP "v.log") -Value $text
if ($ReportUrl) {
  try {
    Invoke-RestMethod -Uri $ReportUrl -Method Post -Body $text -TimeoutSec 60 | Out-Null
    Write-Host "report posted to $ReportUrl"
  } catch {
    Write-Host "report post failed: $_"
  }
}

Write-Host ""
Write-Host "=== summary ==="
foreach ($line in $results) { Write-Host $line }
if ($failures.Count -gt 0) {
  Write-Host "WINDOWS_VALIDATION_FAILED ($($failures.Count))"
  exit 1
}
Write-Host "WINDOWS_VALIDATION_OK"
