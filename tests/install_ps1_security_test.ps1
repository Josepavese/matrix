# Security tests for install/install.ps1.
#
# The installer downloads a release archive and unpacks it, so two guards carry
# the security weight: checksum verification of the downloaded archive, and
# rejection of archive entries that escape the extraction directory. This script
# extracts those two functions from the installer's AST and exercises them
# directly, which runs on any platform PowerShell Core supports.

$ErrorActionPreference = "Stop"

$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$installer = Join-Path (Split-Path -Parent $here) "install/install.ps1"
if (-not (Test-Path $installer)) {
  throw "installer not found at $installer"
}

$failures = New-Object System.Collections.Generic.List[string]

function Assert-Throws {
  param([string]$Name, [scriptblock]$Body)
  try {
    & $Body | Out-Null
  } catch {
    Write-Host "  ok   $Name"
    return
  }
  $failures.Add("$Name did not throw")
  Write-Host "  FAIL $Name did not throw"
}

function Assert-Passes {
  param([string]$Name, [scriptblock]$Body)
  try {
    & $Body | Out-Null
  } catch {
    $failures.Add("$Name threw: $_")
    Write-Host "  FAIL $Name threw: $_"
    return
  }
  Write-Host "  ok   $Name"
}

Write-Host "Parsing $installer"
$tokens = $null
$errors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile($installer, [ref]$tokens, [ref]$errors)
if ($errors.Count -gt 0) {
  foreach ($error in $errors) {
    $failures.Add("parse error: $($error.Message)")
    Write-Host "  FAIL parse error: $($error.Message)"
  }
} else {
  Write-Host "  ok   no parse errors"
}

# Pull the two functions out of the file and define them in this session.
$wanted = @("Test-MatrixChecksum", "Test-MatrixZipArchive")
foreach ($name in $wanted) {
  $functionAst = $ast.Find({
      param($node)
      $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name
    }, $true)
  if ($null -eq $functionAst) {
    # Fall back to the scriptblock inside the assignment form when a function is
    # declared without the function keyword.
    $functionAst = $ast.Find({
        param($node)
        $node -is [System.Management.Automation.Language.AssignmentStatementAst] -and
        $node.Left.Extent.Text -eq $name
      }, $true)
  }
  if ($null -eq $functionAst) {
    $failures.Add("$name not found in the installer")
    Write-Host "  FAIL $name not found in the installer"
    continue
  }
  Invoke-Expression $functionAst.Extent.Text
  Write-Host "  ok   loaded $name"
}

$work = Join-Path ([System.IO.Path]::GetTempPath()) ("matrix-ps1-tests-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $work | Out-Null

try {
  Add-Type -AssemblyName System.IO.Compression.FileSystem

  # --- checksum guard -------------------------------------------------------
  $archive = Join-Path $work "matrix.zip"
  [System.IO.File]::WriteAllText($archive, "payload")
  $good = (Get-FileHash -Algorithm SHA256 -Path $archive).Hash.ToLowerInvariant()
  $checksums = Join-Path $work "checksums.txt"
  # The published file lists two columns: "<hash>  <asset name>".
  [System.IO.File]::WriteAllText($checksums, "$good  matrix_0.0.0_windows_amd64.zip`n")

  Write-Host "checksum guard"
  Assert-Passes "accepts the matching checksum" {
    Test-MatrixChecksum -ChecksumFile $checksums -ArchivePath $archive -AssetName "matrix_0.0.0_windows_amd64.zip"
  }
  Assert-Throws "rejects a tampered archive" {
    $tampered = Join-Path $work "tampered.zip"
    [System.IO.File]::WriteAllText($tampered, "tampered")
    Test-MatrixChecksum -ChecksumFile $checksums -ArchivePath $tampered -AssetName "matrix_0.0.0_windows_amd64.zip"
  }
  Assert-Throws "rejects an asset missing from checksums.txt" {
    Test-MatrixChecksum -ChecksumFile $checksums -ArchivePath $archive -AssetName "matrix_0.0.0_windows_arm64.zip"
  }

  # --- zip-slip guard -------------------------------------------------------
  Write-Host "archive path guard"
  $safeZip = Join-Path $work "safe.zip"
  $safeDir = Join-Path $work "safe"
  New-Item -ItemType Directory -Force -Path (Join-Path $safeDir "bin") | Out-Null
  [System.IO.File]::WriteAllText((Join-Path $safeDir "bin/matrix.exe"), "binary")
  [System.IO.Compression.ZipFile]::CreateFromDirectory($safeDir, $safeZip)
  Assert-Passes "accepts a normal archive" {
    Test-MatrixZipArchive -ArchivePath $safeZip
  }

  foreach ($entry in @("../evil.exe", "/absolute.exe", "C:/windows/evil.exe", "dir/../../evil.exe")) {
    $hostile = Join-Path $work ("hostile-" + ([guid]::NewGuid().ToString("N")) + ".zip")
    $stream = [System.IO.File]::Create($hostile)
    $zip = New-Object System.IO.Compression.ZipArchive($stream, [System.IO.Compression.ZipArchiveMode]::Create)
    try {
      $zipEntry = $zip.CreateEntry($entry)
      $writer = New-Object System.IO.StreamWriter($zipEntry.Open())
      $writer.Write("evil")
      $writer.Dispose()
    } finally {
      $zip.Dispose()
      $stream.Dispose()
    }
    Assert-Throws "rejects archive entry '$entry'" {
      Test-MatrixZipArchive -ArchivePath $hostile
    }
  }
} finally {
  Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}

if ($failures.Count -gt 0) {
  Write-Host ""
  Write-Host "INSTALL_PS1_TESTS_FAILED ($($failures.Count))"
  foreach ($failure in $failures) {
    Write-Host " - $failure"
  }
  exit 1
}

Write-Host ""
Write-Host "INSTALL_PS1_TESTS_OK"
