# demo.ps1 — Interactive demo script for faculty presentation
# Pre-requisite: deploy.ps1 has been run successfully
param(
    [string]$Namespace = "ddos-demo"
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

function Wait-KeyPress {
    param([string]$Message = "Press any key to continue...")
    Write-Host "`n$Message" -ForegroundColor DarkGray
    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
    Write-Host ""
}

# Get Grafana URL
$grafanaIP = kubectl get svc grafana -n $Namespace -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>$null
if ([string]::IsNullOrEmpty($grafanaIP)) {
    Write-Host "ERROR: Grafana LoadBalancer IP not found. Is the system deployed?" -ForegroundColor Red
    exit 1
}

# ──────────────────────────────────────────────────────────────
# Phase A: Show Normal State
# ──────────────────────────────────────────────────────────────
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  DDoS Detection System - Live Demo"     -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Architecture:" -ForegroundColor Yellow
Write-Host "  3 IoT nodes  -->  HLL++ Sketches  -->  Aggregator"
Write-Host "  Each node runs an ensemble detector: LODA + HST + ZScore + EWMA"
Write-Host "  Node 1: No rate limiting (shows raw attack impact)"
Write-Host "  Node 2,3: Rate limiting active (shows mitigation)"
Write-Host ""
Write-Host "Open Grafana in your browser:" -ForegroundColor Green
Write-Host "  http://${grafanaIP}:3000" -ForegroundColor White
Write-Host ""
Write-Host "Current pod status:"
kubectl get pods -n $Namespace --no-headers | ForEach-Object { Write-Host "  $_" }
Write-Host ""
Write-Host "Observe the dashboard:" -ForegroundColor Yellow
Write-Host "  - Unique IPs: ~50/window across all nodes (normal baseline)"
Write-Host "  - Anomaly scores: near 0 (no anomaly)"
Write-Host "  - State timeline: all GREEN (NORMAL)"
Write-Host "  - NSG Firewall: OPEN (green)"

Wait-KeyPress "Press any key to trigger the DDoS attack..."

# ──────────────────────────────────────────────────────────────
# Phase B: Trigger Attack
# ──────────────────────────────────────────────────────────────
Write-Host "--- Triggering DDoS Attack ---" -ForegroundColor Red
Write-Host "  Attack: 10,000 unique IPs/sec on nodes 2 and 3"
Write-Host "  Normal traffic continues on all nodes"
Write-Host ""

# Clean up any previous attack jobs
kubectl delete jobs attack-node-2 attack-node-3 -n $Namespace 2>$null

# Apply attack with ACR substitution
$acrLoginServer = kubectl get deployment iot-node-1 -n $Namespace -o jsonpath='{.spec.template.spec.containers[0].image}' 2>$null
$acrLoginServer = $acrLoginServer -replace '/ddos-agent:latest', ''
(Get-Content "$scriptDir\05-attack.yaml" -Raw) -replace '__ACR__', $acrLoginServer | kubectl apply -f -

Write-Host ""
Write-Host "Attack jobs started! Watch Grafana..." -ForegroundColor Red
Write-Host ""

# ──────────────────────────────────────────────────────────────
# Phase C: Narrate
# ──────────────────────────────────────────────────────────────
Write-Host "What to watch (in order):" -ForegroundColor Yellow
Write-Host "  1. [~30s] Attack begins - Unique IPs spike on node-2 and node-3"
Write-Host "  2. [~33s] Ensemble score crosses 0.6 threshold"
Write-Host "  3. [~35s] State machine transitions to UNDER ATTACK (red)"
Write-Host "  4. [~35s] Rate limiter activates on node-2, node-3 - drops appear"
Write-Host "  5. [~35s] Node-1 has NO rate limiting - compare the impact"
Write-Host "  6. [~36s] Aggregator sees majority under attack -> GLOBAL DEFENSE"
Write-Host "  7. [~38s] NSG Firewall: LOCKED DOWN (red) - Azure NSG rule created"
Write-Host "  8. [~90s] Attack ends - scores drop - RECOVERY state"
Write-Host "  9. [~95s] NSG Firewall: OPEN (green) - lockdown rule removed"
Write-Host ""
Write-Host "Key insight:" -ForegroundColor Cyan
Write-Host "  All detection uses HLL++ sketches - NO IP addresses stored!"
Write-Host "  Memory per node: ~16 KB (vs gigabytes for raw IP lists)"
Write-Host "  Detection: ML ensemble (LODA + HST + ZScore + EWMA)"
Write-Host "  Response: Local rate limiting + Azure NSG lockdown"

Wait-KeyPress "Press any key after attack completes..."

# ──────────────────────────────────────────────────────────────
# Phase D: Post-attack
# ──────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "--- Post-Attack Status ---" -ForegroundColor Green
Write-Host ""
kubectl get pods -n $Namespace --no-headers | ForEach-Object { Write-Host "  $_" }

Write-Host ""
Write-Host "Options:" -ForegroundColor Yellow
Write-Host "  [R] Re-trigger attack"
Write-Host "  [Q] End demo"
Write-Host ""

$choice = Read-Host "Choice"
if ($choice -eq "R" -or $choice -eq "r") {
    Write-Host "Re-triggering attack..."
    kubectl delete jobs attack-node-2 attack-node-3 -n $Namespace 2>$null
    (Get-Content "$scriptDir\05-attack.yaml" -Raw) -replace '__ACR__', $acrLoginServer | kubectl apply -f -
    Write-Host "Attack re-triggered! Watch Grafana." -ForegroundColor Red
}

Write-Host "`nDemo complete." -ForegroundColor Green
