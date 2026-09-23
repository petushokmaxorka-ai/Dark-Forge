# DARK FORGE - Windows launcher.
# 1. Starts forge.exe backend on :9091 (if not already running).
# 2. Prints instructions for the VSCodium extension.
#
# Works both from a release package (launch-windows.ps1 next to forge\) and
# from a source checkout (launcher\launch-windows.ps1).
#
# Usage: powershell -ExecutionPolicy Bypass -File launch-windows.ps1 [-Workspace path]

param(
    [string]$Workspace = (Get-Location).Path
)

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = if (Test-Path (Join-Path $ScriptDir "forge")) { $ScriptDir } else { Split-Path -Parent $ScriptDir }
$ForgeBin = Join-Path $Root "forge\forge.exe"
# $env:DARKFORGE_CONFIG, then the user's config, then the bundled example.
$Config = $env:DARKFORGE_CONFIG
if (-not $Config) {
    $Config = @(
        (Join-Path $HOME ".config\dark-forge\forge.yaml"),
        (Join-Path $Root "forge.yaml"),
        (Join-Path $Root "config\forge.example.yaml"),
        (Join-Path $Root "forge.example.yaml")
    ) | Where-Object { Test-Path $_ } | Select-Object -First 1
}

function Test-Forge {
    try {
        $r = Invoke-WebRequest -Uri "http://127.0.0.1:9091/api/status" -TimeoutSec 2 -UseBasicParsing
        return ($r.StatusCode -eq 200)
    } catch { return $false }
}

if (-not (Test-Forge)) {
    if (Test-Path $ForgeBin) {
        Write-Host "Dark Forge: starting backend on :9091..."
        $ForgeArgs = @("--repo", "`"$Workspace`"")
        if ($Config) { $ForgeArgs += @("--config", "`"$Config`"") }
        $proc = Start-Process -FilePath $ForgeBin -ArgumentList $ForgeArgs -WindowStyle Hidden -PassThru
        for ($i = 0; $i -lt 15; $i++) {
            Start-Sleep -Seconds 1
            if (Test-Forge) { break }
        }
        if (Test-Forge) { Write-Host "Backend up (pid $($proc.Id))." }
        else { Write-Warning "Backend did not answer yet - run $ForgeBin in a terminal to see its output" }
    } else {
        Write-Warning "forge.exe not found at $ForgeBin"
    }
} else {
    Write-Host "Dark Forge: backend already running on :9091"
}

Write-Host ""
Write-Host "Next steps (VSCodium integration):"
Write-Host "  1. Open VSCodium (or the Dark Forge IDE, which bundles the extensions)"
Write-Host "  2. Extensions -> ... -> Install from VSIX -> extensions\swarm-chat-0.2.0.vsix"
Write-Host "  3. Settings -> 'darkforge.baseUrl' -> http://127.0.0.1:9091"
Write-Host "  4. Activity bar -> Dark Forge -> Swarm Chat"
