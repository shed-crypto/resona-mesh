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

# Ensure Go binary is in PATH
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    if (Test-Path "C:\Program Files\Go\bin") {
        $env:PATH += ";C:\Program Files\Go\bin"
    } elseif (Test-Path "C:\Go\bin") {
        $env:PATH += ";C:\Go\bin"
    }
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
Write-Host "`n[3/3] Compiling standalone binary (resonamesh.exe)..." -ForegroundColor Yellow

$embeddedSeeds = ""
if (Test-Path "bootstrap.local.json") {
    try {
        $jsonContent = Get-Content "bootstrap.local.json" -Raw | ConvertFrom-Json
        if ($jsonContent -is [Array]) {
            $embeddedSeeds = ($jsonContent -join ",")
            Write-Host "Embedding local bootstrap seeds from bootstrap.local.json ($($jsonContent.Count) endpoints)" -ForegroundColor Cyan
        }
    } catch {
        Write-Host "Warning: Could not parse bootstrap.local.json, skipping embedding." -ForegroundColor Yellow
    }
}

$embeddedRecoveryPubKey = ""
$embeddedReleasePubKey = ""
if (Test-Path "trust.local.json") {
    try {
        $trustJson = Get-Content "trust.local.json" -Raw | ConvertFrom-Json
        if ($trustJson.recovery_pubkey) {
            $embeddedRecoveryPubKey = $trustJson.recovery_pubkey
            Write-Host "Embedding Recovery Public Key from trust.local.json" -ForegroundColor Cyan
        }
        if ($trustJson.release_pubkey) {
            $embeddedReleasePubKey = $trustJson.release_pubkey
            Write-Host "Embedding Release Public Key from trust.local.json" -ForegroundColor Cyan
        }
    } catch {
        Write-Host "Warning: Could not parse trust.local.json, skipping trust key embedding." -ForegroundColor Yellow
    }
}

$ldflags = "-s -w -X main.AppVersion=0.1.0-alpha -X main.AppName=ResonaMesh"
if ($embeddedRecoveryPubKey -ne "") {
    $ldflags += " -X main.RecoveryPubKey=$embeddedRecoveryPubKey"
}
if ($embeddedReleasePubKey -ne "") {
    $ldflags += " -X main.ReleasePubKey=$embeddedReleasePubKey"
}
if ($embeddedSeeds -ne "") {
    $ldflags += " -X main.DefaultBootstrapSeeds=$embeddedSeeds"
}

go build -ldflags $ldflags -o resonamesh.exe ./cmd/resonamesh

if (Test-Path "resonamesh.exe") {
    $size = (Get-Item "resonamesh.exe").Length / 1MB
    Write-Host "`nSUCCESS: resonamesh.exe generated ($([math]::Round($size, 2)) MB)" -ForegroundColor Green
    Write-Host "Run with: .\resonamesh.exe --http 127.0.0.1:8080 --p2p 4001" -ForegroundColor Cyan
} else {
    Write-Error "Build failed: resonamesh.exe was not created."
}
