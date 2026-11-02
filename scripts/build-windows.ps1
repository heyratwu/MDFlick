param(
    [ValidateSet('amd64', 'arm64')]
    [string]$Arch = 'amd64'
)
$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '..')
$staging = Join-Path (Get-Location) ('.windows-build.' + [guid]::NewGuid().ToString('N'))
try {
    # On Windows MyGo downloads and verifies NSIS if it is not installed.
    & go tool mygo build -platform "windows/$Arch" -o $staging
    if ($LASTEXITCODE -ne 0) { throw 'Windows build failed.' }
    $output = Join-Path $staging "windows-$Arch"
    $installers = @(Get-ChildItem -LiteralPath $output -Filter "*-setup-*-$Arch.exe")
    if ($installers.Count -ne 1) { throw 'Expected exactly one Windows installer.' }
    $destination = Join-Path 'build' "windows-$Arch"
    New-Item -ItemType Directory -Force -Path $destination | Out-Null
    Get-ChildItem -LiteralPath $output | Copy-Item -Destination $destination -Recurse -Force
    Write-Host "Built $destination/$($installers[0].Name)"
} finally {
    if (Test-Path -LiteralPath $staging) { Remove-Item -LiteralPath $staging -Recurse -Force }
    Pop-Location
}
