# DARK FORGE - Windows launcher.
# 1. Starts forge.exe backend on :9091 (if not already running).
# 2. Prints instructions for the VSCodium extension (full IDE ships in v1.1).
#
# Usage: powershell -ExecutionPolicy Bypass -File launch-windows.ps1 [-Workspace path]

param(
    [string]$Workspace = (Get-Location).Path
)

$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$ForgeBin = Join-Path $Root "forge\forge.exe"
$Config = if ($env:DARKFORGE_CONFIG) { $env:DARKFORGE_CONFIG } else { Join-Path $Root "config\forge.example.yaml" }

function Test-Forge {
    try {
        $r = Invoke-WebRequest -Uri "http://127.0.0.1:9091/api/status" -TimeoutSec 2 -UseBasicParsing
        return ($r.StatusCode -eq 200)
    } catch { return $false }
}

if (-not (Test-Forge)) {
    if (Test-Path $ForgeBin) {
        Write-Host "Dark Forge: starting backend on :9091..."
        $proc = Start-Process -FilePath $ForgeBin -ArgumentList "--config", "`"$Config`"", "--repo", "`"$Workspace`"" -WindowStyle Hidden -PassThru
        for ($i = 0; $i -lt 15; $i++) {
            Start-Sleep -Seconds 1
            if (Test-Forge) { break }
        }
        if (Test-Forge) { Write-Host "Backend up (pid $($proc.Id))." }
        else { Write-Warning "Backend did not answer yet - check forge\forge.log" }
    } else {
        Write-Warning "forge.exe not found at $ForgeBin"
    }
} else {
    Write-Host "Dark Forge: backend already running on :9091"
}

Write-Host ""
Write-Host "Next steps (VSCodium integration):"
Write-Host "  1. Open VSCodium"
Write-Host "  2. Extensions -> ... -> Install from VSIX -> swarm-chat-0.2.0.vsix"
Write-Host "  3. Settings -> 'darkforge.baseUrl' -> http://127.0.0.1:9091"
Write-Host "  4. Activity bar -> Dark Forge -> Swarm Chat"
