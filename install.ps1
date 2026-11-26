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

# Get latest version: where github.com/$repo/releases/latest points, the
# redirect not followed (as ccdrawu's), never a pre-release. Not GitHub's API:
# to a network not signed in it answers 60 times an hour, then says 403.
Write-Host "Fetching latest release..."
Add-Type -AssemblyName System.Net.Http
$handler = New-Object System.Net.Http.HttpClientHandler
$handler.AllowAutoRedirect = $false
$client = New-Object System.Net.Http.HttpClient($handler)
try {
    $location = $client.GetAsync("https://github.com/$repo/releases/latest").Result.Headers.Location
} catch {
    $location = $null
} finally {
    $client.Dispose()
}
if (-not $location -or "$location" -notmatch "/releases/tag/(v?([^/]+))$") {
    Write-Host "Error: no release of $app found at https://github.com/$repo/releases" -ForegroundColor Red
    exit 1
}
$tag = $Matches[1]
$version = $Matches[2]
Write-Host "Latest version: $version"

# Download
$fileName = "${app}_${version}_windows_${arch}.zip"
$downloadUrl = "https://github.com/$repo/releases/download/$tag/$fileName"
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
