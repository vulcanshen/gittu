# gittu installer for Windows.
# Usage: irm https://raw.githubusercontent.com/vulcanshen/gittu/main/install.ps1 | iex
#
# gittu's own (harness project.2: an app with a Windows build writes it): the
# same steps as install.sh, into %LOCALAPPDATA%\gittu.

$ErrorActionPreference = "Stop"

$app = "gittu"
$repo = "vulcanshen/$app"
$installDir = Join-Path $env:LOCALAPPDATA $app

# Detect architecture
if (-not [Environment]::Is64BitOperatingSystem) {
    Write-Host "Error: 32-bit Windows is not supported." -ForegroundColor Red
    exit 1
}
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }

# Get latest version
Write-Host "Fetching latest release..."
$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
$version = $release.tag_name -replace "^v", ""
Write-Host "Latest version: $version"

# Download
$fileName = "${app}_${version}_windows_${arch}.zip"
$downloadUrl = "https://github.com/$repo/releases/download/$($release.tag_name)/$fileName"
$tempDir = Join-Path $env:TEMP "$app-install"
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
$tempFile = Join-Path $tempDir $fileName
Write-Host "Downloading $fileName..."
Invoke-WebRequest -Uri $downloadUrl -OutFile $tempFile -UseBasicParsing

# Extract and install: only the program is replaced
Expand-Archive -Path $tempFile -DestinationPath $tempDir -Force
New-Item -ItemType Directory -Path $installDir -Force | Out-Null
Copy-Item (Join-Path $tempDir "$app.exe") (Join-Path $installDir "$app.exe") -Force
Remove-Item $tempDir -Recurse -Force

Write-Host ""
Write-Host "$app $version installed to $installDir"

# Add the install dir to the user's PATH
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    Write-Host ""
    Write-Host "Added $installDir to your PATH: open a new terminal to use it."
}

Write-Host ""
Write-Host "NOTE: $app draws with Nerd Font glyphs. Use a Nerd Font terminal"
Write-Host "      profile (https://www.nerdfonts.com) or they render as boxes."
Write-Host "      It also needs a truecolor (24-bit) terminal, such as Windows"
Write-Host "      Terminal; with 256 colours the dimmed and layered colours"
Write-Host "      can't be told apart."
Write-Host ""
Write-Host "It runs the git you have installed: Git for Windows must be on your PATH."
Write-Host ""
Write-Host "Run '$app' in a git repository to start."
