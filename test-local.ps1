#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Test 1: Local single-node DDoS detection.

.DESCRIPTION
    Port-forwards the agent's Prometheus metrics port locally, then polls
    ddos_attack_status and ddos_unique_ips_current_window every 2 seconds.
    Run syn_flood.py in a separate terminal to trigger the attack.

.USAGE
    # Terminal 1 – start this watcher:
    .\test-local.ps1

    # Terminal 2 – send the flood (needs Scapy + Npcap):
    python syn_flood.py
#>

$AGENT_PORT     = 9091
$POLL_INTERVAL  = 2   # seconds
$METRICS_URL    = "http://localhost:$AGENT_PORT/metrics"

Write-Host "[*] Starting port-forward: agent :9090 -> localhost:$AGENT_PORT" -ForegroundColor Cyan
$pfJob = Start-Job -ScriptBlock {
    kubectl port-forward daemonset/ddos-agent 9091:9090 2>&1
}

Start-Sleep -Seconds 3

Write-Host "[*] Polling agent metrics every ${POLL_INTERVAL}s (Ctrl+C to stop)" -ForegroundColor Cyan
Write-Host "    Run 'python syn_flood.py' in another terminal to trigger detection`n"

try {
    while ($true) {
        try {
            $raw = Invoke-WebRequest -Uri $METRICS_URL -UseBasicParsing -TimeoutSec 3 -ErrorAction Stop
            $lines = $raw.Content -split "`n"

            $status = ($lines | Where-Object { $_ -match '^ddos_attack_status\s' }) -replace '.*\s', ''
            $ips    = ($lines | Where-Object { $_ -match '^ddos_unique_ips_current_window\s' }) -replace '.*\s', ''
            $mem    = ($lines | Where-Object { $_ -match '^ddos_memory_usage_bytes\s' }) -replace '.*\s', ''

            $ts = Get-Date -Format "HH:mm:ss"
            $color = if ($status -eq "1") { "Red" } else { "Green" }
            $attackLabel = if ($status -eq "1") { " *** ATTACK DETECTED ***" } else { "" }

            Write-Host "[$ts] unique_ips=$ips  attack_status=$status  memory=${mem}B$attackLabel" -ForegroundColor $color
        } catch {
            Write-Host "[!] Could not reach metrics endpoint: $_" -ForegroundColor Yellow
        }
        Start-Sleep -Seconds $POLL_INTERVAL
    }
} finally {
    Write-Host "`n[*] Stopping port-forward..."
    Stop-Job $pfJob
    Remove-Job $pfJob
}
