#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Test 2: Cluster-level DDoS detection via the aggregator.

.DESCRIPTION
    Port-forwards the aggregator's Prometheus metrics port locally, then polls
    the cluster-level ddos_attack_status every 2 seconds.

    The attack flow:
      Windows NIC  ─► (Npcap/pcap)  ─► ddos-agent Pod (hostNetwork)
                                                │  SubmitWindow gRPC every 10 s
                                                ▼
                                     ddos-aggregator Pod
                                     (merges sketches, detects cluster attack)

    For a simulated distributed attack (two agents flooding at half the threshold
    each), apply the second-agent manifest:
        kubectl apply -f k8s\second-agent-deploy.yaml
    which deploys a Deployment-based second agent alongside the DaemonSet agent.

.USAGE
    # Terminal 1 – start this watcher:
    .\test-cluster.ps1

    # Terminal 2 – send the flood:
    python syn_flood.py
#>

$AGG_PORT      = 9092
$POLL_INTERVAL = 2   # seconds
$METRICS_URL   = "http://localhost:$AGG_PORT/metrics"

Write-Host "[*] Starting port-forward: aggregator :9090 -> localhost:$AGG_PORT" -ForegroundColor Cyan
$pfJob = Start-Job -ScriptBlock {
    kubectl port-forward deployment/ddos-aggregator 9092:9090 2>&1
}

Start-Sleep -Seconds 3

Write-Host "[*] Polling aggregator metrics every ${POLL_INTERVAL}s (Ctrl+C to stop)" -ForegroundColor Cyan
Write-Host "    Run 'python syn_flood.py' in another terminal to trigger detection`n"
Write-Host "    Agent pushes a sketch every 10 s, so detection may lag up to one window.`n"

try {
    while ($true) {
        try {
            $raw = Invoke-WebRequest -Uri $METRICS_URL -UseBasicParsing -TimeoutSec 3 -ErrorAction Stop
            $lines = $raw.Content -split "`n"

            $status = ($lines | Where-Object { $_ -match '^ddos_attack_status\s' }) -replace '.*\s', ''
            $ips    = ($lines | Where-Object { $_ -match '^ddos_unique_ips_current_window\s' }) -replace '.*\s', ''

            $ts = Get-Date -Format "HH:mm:ss"
            $color = if ($status -eq "1") { "Red" } else { "Green" }
            $attackLabel = if ($status -eq "1") { " *** CLUSTER ATTACK DETECTED ***" } else { "" }

            Write-Host "[$ts] cluster_unique_ips=$ips  cluster_attack_status=$status$attackLabel" -ForegroundColor $color
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
