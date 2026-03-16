#Requires -Version 5.1
<#
.SYNOPSIS
    Installs the Intentra CLI on Windows.
.DESCRIPTION
    Downloads and installs the latest version of the Intentra CLI.
.EXAMPLE
    irm https://install.intentra.sh/install.ps1 | iex
#>

$ErrorActionPreference = "Stop"

$GitHubOwner = "intentrahq"
$GitHubRepo = "intentra-cli"
$BinaryName = "intentra"

function Get-Architecture {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        "X64" { return "amd64" }
        "Arm64" { return "arm64" }
        default {
            Write-Error "Unsupported architecture: $arch"
            exit 1
        }
    }
}

function Get-LatestVersion {
    Write-Host "Fetching latest version..."
    $releasesUrl = "https://api.github.com/repos/$GitHubOwner/$GitHubRepo/releases/latest"
    
    try {
        $release = Invoke-RestMethod -Uri $releasesUrl -UseBasicParsing
        return $release.tag_name
    }
    catch {
        Write-Error "Failed to fetch latest version: $_"
        exit 1
    }
}

function Get-InstallDirectory {
    $installDir = Join-Path $env:LOCALAPPDATA "intentra"
    if (-not (Test-Path $installDir)) {
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    }
    return $installDir
}

function Add-ToPath {
    param([string]$Directory)
    
    $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($currentPath -notlike "*$Directory*") {
        Write-Host "Adding $Directory to PATH..."
        $newPath = "$currentPath;$Directory"
        [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
        $env:PATH = "$env:PATH;$Directory"
        Write-Host "PATH updated. You may need to restart your terminal for changes to take effect."
    }
}

function Get-Checksum {
    param([string]$FilePath)
    
    $hash = Get-FileHash -Path $FilePath -Algorithm SHA256
    return $hash.Hash.ToLower()
}

function Install-Intentra {
    $arch = Get-Architecture
    $version = Get-LatestVersion
    $versionNum = $version.TrimStart("v")
    
    Write-Host "Installing Intentra CLI $version for windows/$arch..."
    
    $archiveName = "${BinaryName}_${versionNum}_windows_${arch}.zip"
    $downloadUrl = "https://github.com/$GitHubOwner/$GitHubRepo/releases/download/$version/$archiveName"
    $checksumUrl = "https://github.com/$GitHubOwner/$GitHubRepo/releases/download/$version/checksums.txt"
    
    $tempDir = Join-Path $env:TEMP "intentra-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
    
    try {
        $archivePath = Join-Path $tempDir $archiveName
        $checksumPath = Join-Path $tempDir "checksums.txt"
        
        Write-Host "Downloading $archiveName..."
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing
        
        Write-Host "Downloading checksums..."
        Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath -UseBasicParsing
        
        Write-Host "Verifying checksum..."
        $checksums = Get-Content $checksumPath
        $expectedLine = $checksums | Where-Object { $_ -like "*$archiveName*" }
        
        if ($expectedLine) {
            $expectedChecksum = ($expectedLine -split "\s+")[0].ToLower()
            $actualChecksum = Get-Checksum -FilePath $archivePath
            
            if ($expectedChecksum -ne $actualChecksum) {
                Write-Error "Checksum verification failed!`nExpected: $expectedChecksum`nActual: $actualChecksum"
                exit 1
            }
            Write-Host "Checksum verified"
        }
        else {
            Write-Warning "Could not find checksum for $archiveName, skipping verification"
        }
        
        Write-Host "Extracting archive..."
        Expand-Archive -Path $archivePath -DestinationPath $tempDir -Force
        
        $installDir = Get-InstallDirectory
        $binaryPath = Join-Path $installDir "$BinaryName.exe"
        
        Write-Host "Installing to $installDir..."
        $extractedBinary = Join-Path $tempDir "$BinaryName.exe"
        Copy-Item -Path $extractedBinary -Destination $binaryPath -Force
        
        Add-ToPath -Directory $installDir
        
        Write-Host ""
        Write-Host "============================================" -ForegroundColor Green
        Write-Host "  Intentra CLI installed successfully!" -ForegroundColor Green
        Write-Host "============================================" -ForegroundColor Green
        Write-Host ""
        Write-Host "Version: $version"
        Write-Host "Location: $binaryPath"
        Write-Host ""
        Write-Host "Get started:"
        Write-Host "  intentra --help"
        Write-Host "  intentra login"
        Write-Host "  intentra install cursor"
        Write-Host ""
        Write-Host "Documentation: https://intentra.sh/docs"
        Write-Host ""
        
        if (-not (Get-Command $BinaryName -ErrorAction SilentlyContinue)) {
            Write-Warning "Please restart your terminal for PATH changes to take effect."
        }
    }
    finally {
        if (Test-Path $tempDir) {
            Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

Install-Intentra
