$ErrorActionPreference = "Stop"

$repo = "kint-pro/kint-vault-cli"
$binary = "kint-vault.exe"
$installDir = "$env:LOCALAPPDATA\kint-vault"

$arch = if ([Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else {
    Write-Error "Unsupported: 32-bit systems"; exit 1
}

$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
$version = $release.tag_name -replace "^v", ""
$asset = "kint-vault-cli_${version}_windows_${arch}.zip"
$url = $release.assets | Where-Object { $_.name -eq $asset } | Select-Object -ExpandProperty browser_download_url

if (-not $url) { Write-Error "No release found for windows/$arch"; exit 1 }

$tmp = New-TemporaryFile | Rename-Item -NewName { $_.Name + ".zip" } -PassThru
Write-Host "Downloading kint-vault v$version (windows/$arch)..."
Invoke-WebRequest -Uri $url -OutFile $tmp

$extractDir = Join-Path $env:TEMP "kint-vault-install"
if (Test-Path $extractDir) { Remove-Item $extractDir -Recurse -Force }
Expand-Archive -Path $tmp -DestinationPath $extractDir
Remove-Item $tmp

if (-not (Test-Path $installDir)) { New-Item -ItemType Directory -Path $installDir | Out-Null }
Move-Item -Path (Join-Path $extractDir $binary) -Destination (Join-Path $installDir $binary) -Force
Remove-Item $extractDir -Recurse -Force

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    Write-Host "Added $installDir to PATH (restart terminal to take effect)"
}

Write-Host "Installed kint-vault v$version to $installDir\$binary"
