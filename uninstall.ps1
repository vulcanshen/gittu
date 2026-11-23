# gittu uninstaller for Windows.
# Usage: irm https://raw.githubusercontent.com/vulcanshen/gittu/main/uninstall.ps1 | iex
#
# gittu's own (harness project.2: an app with a Windows build writes it): the
# same steps as uninstall.sh.

$ErrorActionPreference = "Stop"

$app = "gittu"
$installDir = Join-Path $env:LOCALAPPDATA $app
$exe = Join-Path $installDir "$app.exe"

if (-not (Test-Path $exe)) {
    Write-Host "$app not found in $installDir" -ForegroundColor Red
    exit 1
}
Remove-Item $exe -Force
Write-Host "removed $exe"
if (-not (Get-ChildItem $installDir -Force)) {
    Remove-Item $installDir -Force
}

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -contains $installDir) {
    $newPath = ($userPath -split ";" | Where-Object { $_ -ne $installDir }) -join ";"
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "removed $installDir from your PATH"
}

# Ask offers to remove one directory, kept unless the answer is yes.
function Ask($dir, $what) {
    if (-not (Test-Path $dir)) { return }
    $answer = Read-Host "Remove $what in $dir? [y/N]"
    if ($answer -match "^(y|yes)$") {
        Remove-Item $dir -Recurse -Force
        Write-Host "removed $dir"
    } else {
        Write-Host "kept $dir"
    }
}

# Where tdp E3 puts the settings; GITTU__CONFIG moves them. The state lives
# there too unless GITTU__STATE moved it, so it is asked about on its own only
# then.
$configDir = if ($env:GITTU__CONFIG) { $env:GITTU__CONFIG } elseif ($env:XDG_CONFIG_HOME) { Join-Path $env:XDG_CONFIG_HOME $app } else { Join-Path $env:USERPROFILE ".config\$app" }
Ask $configDir "$app settings"
if ($env:GITTU__STATE -and ($env:GITTU__STATE -ne $configDir)) {
    Ask $env:GITTU__STATE "$app state"
}

Write-Host ""
Write-Host "$app uninstalled."
