# ResonaMesh Native Build Script for Windows PowerShell
param (
    [switch]$SkipFrontend = $false,
    [switch]$RunTests = $false,
    [switch]$RunAcceptance = $false
)

$ErrorActionPreference = "Stop"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  ResonaMesh Single-Binary Build System " -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

# 1. Frontend Build
if (-not $SkipFrontend) {
    Write-Host "`n[1/3] Building React + Vite Web UI..." -ForegroundColor Yellow
    Push-Location "web"
    try {
        if (-not (Test-Path "node_modules")) {
            Write-Host "Installing npm dependencies..." -ForegroundColor Gray
            npm install
        }
        npm run build
    } finally {
        Pop-Location
    }
    Write-Host "Frontend compiled into web/dist successfully." -ForegroundColor Green
} else {
    Write-Host "`n[1/3] Skipping frontend build (reusing web/dist)..." -ForegroundColor Gray
}

# 2. Automated Testing
if ($RunTests) {
    Write-Host "`n[2/3] Running Go unit & module test suites..." -ForegroundColor Yellow
    go test ./...
}

if ($RunAcceptance) {
    Write-Host "`n[2/3] Running T01-T16 Acceptance test suite..." -ForegroundColor Yellow
    go test -v -count=1 ./tests/integration
}

# 3. Compile Standalone Go Binary
Write-Host "`n[3/3] Compiling standalone standalone binary (resonamesh.exe)..." -ForegroundColor Yellow
$ldflags = "-s -w -X main.AppVersion=0.1.0-alpha -X main.AppName=ResonaMesh"
go build -ldflags $ldflags -o resonamesh.exe ./cmd/resonamesh

if (Test-Path "resonamesh.exe") {
    $size = (Get-Item "resonamesh.exe").Length / 1MB
    Write-Host "`nSUCCESS: resonamesh.exe generated ($([math]::Round($size, 2)) MB)" -ForegroundColor Green
    Write-Host "Run with: .\resonamesh.exe --http 127.0.0.1:8080 --p2p 4001" -ForegroundColor Cyan
} else {
    Write-Error "Build failed: resonamesh.exe was not created."
}
