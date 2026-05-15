# Ghost Installation Script for Windows
#
# This script automatically downloads and installs the latest version of Ghost
# from the release server. It downloads the appropriate binary for your Windows
# system architecture (x86_64, ARM64, or i386).
#
# Usage:
#   irm https://install.ghost.build/install.ps1 | iex
#
# With custom parameters:
#   $Version="v1.2.3"; irm https://install.ghost.build/install.ps1 | iex
#   $InstallDir="C:\custom\path"; irm https://install.ghost.build/install.ps1 | iex
#
# Or if saved locally:
#   .\install.ps1 -Version "v1.2.3" -InstallDir "C:\custom\path"
#
# Parameters (all optional):
#   Version           - Specific version to install (e.g., "v1.2.3")
#                       Default: installs the latest version
#
#   InstallDir        - Custom installation directory
#                       Default: $env:LOCALAPPDATA\Programs\Ghost
#
# Requirements:
#   - PowerShell 5.1 or higher
#   - Internet connection for downloading

param(
    [string]$Version = $Version,
    [string]$InstallDir = $InstallDir
)

$ErrorActionPreference = "Stop"

# Force TLS 1.2+ for secure downloads
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# Configuration
$RepoName = "ghost"
$BinaryName = "ghost.exe"
$DownloadBaseUrl = "https://install.ghost.build"

# Logging functions
function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Blue
}

function Write-Success {
    param([string]$Message)
    Write-Host "[SUCCESS] $Message" -ForegroundColor Green
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[WARN] $Message" -ForegroundColor Yellow
}

function Write-ErrorMsg {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

# Get version (from Version parameter or latest from CloudFront)
function Get-Version {
    if ($Version) {
        Write-Info "Using specified version: $Version"
        return $Version
    }

    $url = "$DownloadBaseUrl/latest.txt"

    # Try to get version from latest.txt file with retry logic
    $maxRetries = 3
    $retryCount = 0
    $backoffSeconds = 1

    while ($retryCount -le $maxRetries) {
        try {
            Write-Info "Fetching latest version..."
            $version = (Invoke-WebRequest -Uri $url -UseBasicParsing).Content.Trim()

            if ([string]::IsNullOrWhiteSpace($version)) {
                throw "latest.txt file is empty"
            }

            Write-Info "Latest version: $version"
            return $version
        }
        catch {
            $retryCount++
            if ($retryCount -le $maxRetries) {
                Write-Warn "Latest version fetch failed, retrying ($retryCount/$maxRetries)..."
                Start-Sleep -Seconds $backoffSeconds
                $backoffSeconds *= 2
            }
            else {
                Write-ErrorMsg "Failed to fetch latest version after $($maxRetries + 1) attempts"
                Write-ErrorMsg "URL: $url"
                Write-ErrorMsg "Error: $_"
                exit 1
            }
        }
    }
}

# Check if a directory is in PATH
function Test-InPath {
    param([string]$Directory)

    $pathDirs = $env:PATH -split ';'
    foreach ($dir in $pathDirs) {
        if ($dir.TrimEnd('\').ToLower() -eq $Directory.TrimEnd('\').ToLower()) {
            return $true
        }
    }
    return $false
}

# Find the best install directory
function Get-InstallDir {
    # If user specified InstallDir, use it (create if needed)
    if ($InstallDir) {
        try {
            if (-not (Test-Path $InstallDir)) {
                New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
            }
            Write-Info "Using user-specified install directory: $InstallDir"
            return $InstallDir
        }
        catch {
            Write-ErrorMsg "Cannot create user-specified install directory: $InstallDir"
            Write-ErrorMsg "Error: $_"
            exit 1
        }
    }

    # Use standard Windows location for user-installed programs
    $defaultInstallDir = "$env:LOCALAPPDATA\Programs\Ghost"
    try {
        if (-not (Test-Path $defaultInstallDir)) {
            New-Item -ItemType Directory -Path $defaultInstallDir -Force | Out-Null
        }
        Write-Info "Install directory: $defaultInstallDir"
        return $defaultInstallDir
    }
    catch {
        Write-ErrorMsg "Cannot create install directory: $defaultInstallDir"
        Write-ErrorMsg "Error: $_"
        Write-ErrorMsg "Please specify a custom installation directory with `$InstallDir parameter"
        exit 1
    }
}

# Get processor architecture
function Get-Architecture {
    $arch = $env:PROCESSOR_ARCHITECTURE

    switch ($arch) {
        "AMD64" { return "x86_64" }
        "ARM64" { return "arm64" }
        "x86" { return "i386" }
        default {
            Write-ErrorMsg "Unsupported architecture: $arch"
            exit 1
        }
    }
}

# Build archive name for Windows
function Get-ArchiveName {
    $arch = Get-Architecture
    return "${RepoName}_Windows_${arch}.zip"
}

# Download file with retry logic
function Get-FileWithRetry {
    param(
        [string]$Url,
        [string]$OutputFile,
        [string]$Description
    )

    $maxRetries = 3
    $retryCount = 0
    $backoffSeconds = 1

    Write-Info "Downloading $Description..."
    Write-Info "URL: $Url"

    while ($retryCount -le $maxRetries) {
        try {
            Invoke-WebRequest -Uri $Url -OutFile $OutputFile -UseBasicParsing
            return
        }
        catch {
            $retryCount++
            if ($retryCount -le $maxRetries) {
                Write-Warn "$Description download failed, retrying ($retryCount/$maxRetries)..."
                Start-Sleep -Seconds $backoffSeconds
                $backoffSeconds *= 2
            }
            else {
                Write-ErrorMsg "Failed to download $Description after $($maxRetries + 1) attempts"
                Write-ErrorMsg "URL: $Url"
                Write-ErrorMsg "Error: $_"
                exit 1
            }
        }
    }
}

# Verify checksum
function Test-Checksum {
    param(
        [string]$Version,
        [string]$Filename,
        [string]$TmpDir
    )

    # Construct individual checksum file URL
    $checksumUrl = "$DownloadBaseUrl/releases/$Version/${Filename}.sha256"
    $checksumFile = Join-Path $TmpDir "${Filename}.sha256"

    # Download checksum file with retry logic
    Get-FileWithRetry -Url $checksumUrl -OutputFile $checksumFile -Description "checksum file"

    Write-Info "Validating checksum for $Filename..."

    # Read expected checksum
    $expectedChecksum = (Get-Content $checksumFile).Trim()

    # Calculate actual checksum
    $actualChecksum = (Get-FileHash -Path (Join-Path $TmpDir $Filename) -Algorithm SHA256).Hash.ToLower()

    if ($actualChecksum -ne $expectedChecksum) {
        Write-ErrorMsg "Checksum validation failed"
        Write-ErrorMsg "Expected: $expectedChecksum"
        Write-ErrorMsg "Actual: $actualChecksum"
        Write-ErrorMsg "For security reasons, installation has been aborted"
        exit 1
    }
}

# Download and verify archive
function Get-Archive {
    param(
        [string]$Version,
        [string]$ArchiveName,
        [string]$TmpDir
    )

    # Construct download URL
    $downloadUrl = "$DownloadBaseUrl/releases/$Version/$ArchiveName"

    # Get architecture for description
    $arch = Get-Architecture

    # Download archive with retry logic
    Get-FileWithRetry -Url $downloadUrl -OutputFile (Join-Path $TmpDir $ArchiveName) -Description "Ghost $Version for Windows $arch"

    # Download and validate checksum
    Write-Info "Verifying file integrity..."
    Test-Checksum -Version $Version -Filename $ArchiveName -TmpDir $TmpDir
}

# Extract archive and return path to binary
function Extract-Archive {
    param(
        [string]$ArchiveName,
        [string]$TmpDir
    )

    Write-Info "Extracting archive..."

    $archivePath = Join-Path $TmpDir $ArchiveName
    Expand-Archive -Path $archivePath -DestinationPath $TmpDir -Force

    $binaryPath = Join-Path $TmpDir $BinaryName

    # Verify binary exists
    if (-not (Test-Path $binaryPath)) {
        Write-ErrorMsg "Binary not found in archive"
        exit 1
    }

    return $binaryPath
}

# Verify installation
function Test-Installation {
    param([string]$InstallDir)

    $binaryPath = Join-Path $InstallDir $BinaryName

    # Check if binary exists at expected location
    if (-not (Test-Path $binaryPath)) {
        Write-ErrorMsg "Installation verification failed: Binary not found at $binaryPath"
        exit 1
    }

    # Test that the binary is executable and get version
    try {
        $installedVersion = & $binaryPath version --bare --version-check=false 2>$null | Select-Object -First 1
        if ($installedVersion) {
            Write-Success "Ghost installed successfully!"
            Write-Success "Version: $installedVersion"
        }
        else {
            Write-Success "Binary installed successfully at $binaryPath"
        }
    }
    catch {
        Write-ErrorMsg "Installation verification failed: Binary exists but is not executable"
        Write-ErrorMsg "Error: $_"
        exit 1
    }

    # Check if install directory is in PATH
    if (-not (Test-InPath $InstallDir)) {
        Write-Info "Adding $InstallDir to your PATH..."

        try {
            $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')

            $newPath = if ($userPath.EndsWith(';')) {
                "$userPath$InstallDir"
            } else {
                "$userPath;$InstallDir"
            }

            [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')

            # Update current session too
            $sessionPath = if ($env:PATH.EndsWith(';')) {
                "$env:PATH$InstallDir"
            } else {
                "$env:PATH;$InstallDir"
            }
            $env:PATH = $sessionPath

            Write-Success "Added $InstallDir to your PATH"
            Write-Info "Change takes effect immediately in this session"
            Write-Info "New terminals will automatically have ghost in PATH"
        }
        catch {
            Write-Warn "Failed to update PATH automatically: $_"
            Write-Warn ""
            Write-Warn "You can add it manually with these commands:"
            Write-Warn "  `$path = [Environment]::GetEnvironmentVariable('Path', 'User')"
            Write-Warn "  [Environment]::SetEnvironmentVariable('Path', `"`$path;$InstallDir`", 'User')"
            Write-Warn ""
            Write-Warn "Or run the binary directly: $binaryPath"
        }
    }
}

# Run `ghost init` to drive the post-install configuration flow (login,
# MCP server installation, shell completions). PATH setup is already handled
# inline by Test-Installation so this run's current PowerShell session can
# immediately pick up ghost — something a Go subprocess can't do for its
# parent. We pass --skip-if-configured so re-runs of the installer don't
# re-prompt the user unnecessarily.
#
# `ghost init` needs an interactive terminal for its multi-select prompts.
# When running under `irm | iex` with redirected stdin (CI, scripted runs,
# etc.), fall back to printing a hint instead so the install still succeeds.
function Invoke-GhostInit {
    param([string]$BinaryPath)

    if ([Console]::IsInputRedirected -or -not [Environment]::UserInteractive) {
        Write-Info "Run '$BinaryPath init' to finish configuring Ghost."
        return
    }

    try {
        & $BinaryPath --version-check=false init --skip-if-configured
    }
    catch {
        Write-Warn "ghost init failed: $_"
        Write-Warn "Run '$BinaryPath init' manually to finish configuring Ghost."
    }
}

# Main installation process
function Install-Ghost {
    Write-Info "Ghost CLI Installation Script"
    Write-Info "=============================="

    # Get version (uses Version parameter or fetches latest)
    $version = Get-Version

    # Find and ensure install directory exists and get its path
    $installDir = Get-InstallDir

    # Create temporary directory
    $tmpDir = Join-Path $env:TEMP "ghost-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        # Build archive name for Windows
        $archiveName = Get-ArchiveName

        # Download and verify the archive
        Get-Archive -Version $version -ArchiveName $archiveName -TmpDir $tmpDir

        # Extract the archive and get binary path
        $binaryPath = Extract-Archive -ArchiveName $archiveName -TmpDir $tmpDir

        # Copy binary to install directory
        # Remove existing binary first to prevent errors related
        # to swapping out a currently executing binary
        Write-Info "Installing to $installDir..."

        # Clean up any old .old files from previous installations
        Get-ChildItem -Path $installDir -Filter "${BinaryName}.old*" -ErrorAction SilentlyContinue |
            ForEach-Object {
                try {
                    Remove-Item $_.FullName -Force -ErrorAction Stop
                }
                catch {
                    # Still in use, ignore
                }
            }

        $targetPath = Join-Path $installDir $BinaryName
        if (Test-Path $targetPath) {
            try {
                # Try to remove it directly first (works if not running)
                Remove-Item $targetPath -Force -ErrorAction Stop
            }
            catch {
                # If locked, rename it instead
                $timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
                $oldPath = "${targetPath}.old.${timestamp}"

                try {
                    Move-Item $targetPath $oldPath -Force -ErrorAction Stop
                    Write-Warn "Existing binary is in use, renamed to: $(Split-Path $oldPath -Leaf)"
                    Write-Info "The old file will be cleaned up on next installation"
                }
                catch {
                    Write-ErrorMsg "Cannot replace binary at $targetPath"
                    Write-ErrorMsg "The binary is locked by another process"
                    Write-ErrorMsg ""
                    Write-ErrorMsg "To fix this:"
                    Write-ErrorMsg "  1. Close all Ghost windows/processes"
                    Write-ErrorMsg "  2. Run: Stop-Process -Name ghost -Force"
                    Write-ErrorMsg "  3. Try the installation again"
                    exit 1
                }
            }
        }
        Copy-Item $binaryPath $targetPath -Force

        # Verify installation
        Test-Installation -InstallDir $installDir

        # Run the post-install configuration flow
        Invoke-GhostInit -BinaryPath $targetPath
    }
    finally {
        # Clean up temporary directory
        if (Test-Path $tmpDir) {
            Remove-Item $tmpDir -Recurse -Force
        }
    }
}

# Run main function
Install-Ghost
