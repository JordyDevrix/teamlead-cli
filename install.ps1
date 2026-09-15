<#
.SYNOPSIS
    teamlead-cli Installer for Windows
.DESCRIPTION
    Installs teamlead standalone CLI on Windows without clashing with
    PowerShell profiles or system environment variables.
.EXAMPLE
    irm https://raw.githubusercontent.com/JordyDevrix/teamlead-cli/main/install.ps1 | iex
#>

[CmdletBinding()]
param (
    [Parameter(Position = 0)]
    [string]$Version = "latest",

    [Parameter()]
    [string]$InstallDir = "$env:LOCALAPPDATA\teamlead\bin",

    [Parameter()]
    [string]$Repo = "JordyDevrix/teamlead-cli",

    [Parameter()]
    [switch]$NoPathUpdate,

    [Parameter()]
    [switch]$Force
)

$ErrorActionPreference = "Stop"

# Use TLS 1.2+
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13

Write-Host ""
Write-Host "🛡️  teamlead-cli Windows Installer" -ForegroundColor Cyan
Write-Host "   Conflict-free multi-agent coordinator for AI coding agents" -ForegroundColor Gray
Write-Host ""

$arch = "amd64"
$archiveName = "teamlead-windows-$arch.zip"

if ($Version -eq "latest") {
    $baseUrl = "https://github.com/$Repo/releases/latest/download"
} else {
    if (-not $Version.StartsWith("v")) {
        $Version = "v$Version"
    }
    $baseUrl = "https://github.com/$Repo/releases/download/$Version"
}

$downloadUrl = "$baseUrl/$archiveName"
$checksumUrl = "$baseUrl/checksums.txt"

$tempGuid = [Guid]::NewGuid().ToString("N")
$tempDir = Join-Path $env:TEMP "teamlead-install-$tempGuid"
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

try {
    $zipPath = Join-Path $tempDir $archiveName
    $checksumPath = Join-Path $tempDir "checksums.txt"

    Write-Host "[INFO] Downloading $archiveName from $downloadUrl..." -ForegroundColor Cyan
    Invoke-WebRequest -Uri $downloadUrl -OutFile $zipPath -UseBasicParsing

    # Verify checksum if available
    try {
        Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath -UseBasicParsing
        $expectedLine = Get-Content $checksumPath | Where-Object { $_ -match [regex]::Escape($archiveName) } | Select-Object -First 1
        if ($expectedLine) {
            $expectedHash = ($expectedLine -split "\s+")[0].Trim().ToLower()
            $actualHash = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash.ToLower()
            if ($actualHash -ne $expectedHash) {
                Write-Error "SHA256 checksum verification failed! Expected: $expectedHash, Actual: $actualHash"
                exit 1
            }
            Write-Host "[OK] SHA256 checksum verified ($($actualHash.Substring(0, 12))...)" -ForegroundColor Green
        }
    } catch {
        Write-Host "[WARN] Checksums file could not be verified; continuing with download." -ForegroundColor Yellow
    }

    Write-Host "[INFO] Extracting archive..." -ForegroundColor Cyan
    $extractDir = Join-Path $tempDir "extracted"
    Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

    $binaryPath = Join-Path $extractDir "teamlead.exe"
    if (-not (Test-Path $binaryPath)) {
        $binaryPath = Get-ChildItem -Path $extractDir -Filter "*teamlead*.exe" -Recurse | Select-Object -First 1 -ExpandProperty FullName
    }

    if (-not $binaryPath -or -not (Test-Path $binaryPath)) {
        Write-Error "Could not find teamlead.exe in extracted archive."
        exit 1
    }

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    $targetPath = Join-Path $InstallDir "teamlead.exe"
    Copy-Item -Path $binaryPath -Destination $targetPath -Force

    # Also copy as teamlead-cli.exe
    $aliasPath = Join-Path $InstallDir "teamlead-cli.exe"
    Copy-Item -Path $binaryPath -Destination $aliasPath -Force

    Write-Host "[OK] Installed teamlead.exe to $targetPath" -ForegroundColor Green

    # Non-destructive PATH check & update (User registry PATH only, zero profile clobbering)
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $normalizedInstallDir = $InstallDir.TrimEnd("\")
    $currentPaths = ($userPath -split ";") | ForEach-Object { $_.Trim().TrimEnd("\") } | Where-Object { -not [string]::IsNullOrEmpty($_) }

    if ($currentPaths -contains $normalizedInstallDir) {
        Write-Host "[OK] $InstallDir is already in your User PATH. No environment changes needed." -ForegroundColor Green
    } elseif ($NoPathUpdate) {
        Write-Host "[INFO] Skipping PATH update (-NoPathUpdate specified)." -ForegroundColor Cyan
        Write-Host "[WARN] Add '$InstallDir' to your PATH manually to run teamlead from any terminal." -ForegroundColor Yellow
    } else {
        Write-Host "[INFO] Adding $InstallDir to User PATH..." -ForegroundColor Cyan
        $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $normalizedInstallDir } else { "$userPath;$normalizedInstallDir" }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        $env:PATH = "$env:PATH;$normalizedInstallDir"
        Write-Host "[OK] Successfully updated User PATH." -ForegroundColor Green
        Write-Host "[INFO] Restart your terminal or PowerShell session for changes to take full effect." -ForegroundColor Gray
    }

    Write-Host ""
    Write-Host "✓ Installation complete!" -ForegroundColor Green
    Write-Host ""
    Write-Host "To get started with teamlead:"
    Write-Host "  1. Initialize in a git repository:  teamlead init" -ForegroundColor Cyan
    Write-Host "  2. Add a scoped task:              teamlead task add `"My Task`" --scope `"src/**`"" -ForegroundColor Cyan
    Write-Host "  3. Launch an agent safely:         teamlead run --agent claude --task T-1 -- claude" -ForegroundColor Cyan
    Write-Host "  4. View live multi-agent board:    teamlead status" -ForegroundColor Cyan
    Write-Host ""
} finally {
    if (Test-Path $tempDir) {
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
