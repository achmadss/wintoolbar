# Builds wintoolbar.
#   .\build.ps1          release build: bin\wintoolbar.exe (no console)
#   .\build.ps1 -Debug   debug build:   bin\wintoolbar-debug.exe (console, symbols)
#   .\build.ps1 -Test    run tests first
param([switch]$Debug, [switch]$Test)
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$env:CGO_ENABLED = '0'

if ($Test) {
    go vet ./...
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
    go test ./...
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
}

New-Item -ItemType Directory -Force bin | Out-Null
if ($Debug) {
    go build -o bin\wintoolbar-debug.exe .
} else {
    go build -trimpath -ldflags "-s -w -H windowsgui" -o bin\wintoolbar.exe .
}
if ($LASTEXITCODE) { exit $LASTEXITCODE }
Get-ChildItem bin\*.exe | ForEach-Object { '{0}  {1:N2} MB' -f $_.Name, ($_.Length / 1MB) }
