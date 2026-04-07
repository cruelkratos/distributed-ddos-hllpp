# deploy.ps1 — Full AKS deployment for DDoS detection system
# Usage: .\deploy.ps1 [-SkipAzureSetup] [-SkipBuild]
param(
    [string]$ResourceGroup = "ddos-demo-rg",
    [string]$AksName = "ddos-cluster",
    [string]$AcrName = "ddosdemoacr",
    [string]$Location = "eastus",
    [switch]$SkipAzureSetup,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectDir = Resolve-Path "$scriptDir\..\.."

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  DDoS Detection System - AKS Deploy"   -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

# ──────────────────────────────────────────────────────────────
# Section 1: Azure Resource Setup (one-time)
# ──────────────────────────────────────────────────────────────
if (-not $SkipAzureSetup) {
    Write-Host "`n--- Azure Resource Setup ---" -ForegroundColor Yellow

    Write-Host "Creating resource group '$ResourceGroup'..."
    az group create -n $ResourceGroup -l $Location -o none

    Write-Host "Creating ACR '$AcrName'..."
    az acr create -g $ResourceGroup -n $AcrName --sku Basic -o none

    Write-Host "Creating AKS cluster '$AksName'..."
    az aks create -g $ResourceGroup -n $AksName `
        --node-count 2 `
        --node-vm-size Standard_B2ms `
        --attach-acr $AcrName `
        --generate-ssh-keys `
        -o none

    Write-Host "Getting AKS credentials..."
    az aks get-credentials -g $ResourceGroup -n $AksName --overwrite-existing
}

# ──────────────────────────────────────────────────────────────
# Section 2: Build & Push Images via ACR
# ──────────────────────────────────────────────────────────────
if (-not $SkipBuild) {
    Write-Host "`n--- Building Images in ACR ---" -ForegroundColor Yellow
    Push-Location $projectDir

    Write-Host "Building ddos-agent..."
    az acr build -r $AcrName -t ddos-agent:latest -f Dockerfile.agent .

    Write-Host "Building ddos-aggregator..."
    az acr build -r $AcrName -t ddos-aggregator:latest -f Dockerfile.aggregator .

    Write-Host "Building ddos-iot-sim..."
    az acr build -r $AcrName -t ddos-iot-sim:latest -f Dockerfile.iot-sim .

    Pop-Location
}

$acrLoginServer = az acr show -n $AcrName --query loginServer -o tsv
Write-Host "ACR login server: $acrLoginServer"

# ──────────────────────────────────────────────────────────────
# Section 3: Deploy K8s Manifests
# ──────────────────────────────────────────────────────────────
Write-Host "`n--- Deploying to AKS ---" -ForegroundColor Yellow

# Apply namespace first
kubectl apply -f "$scriptDir\00-namespace.yaml"

# ──────────────────────────────────────────────────────────────
# Section 3a: Setup NSG Firewall & Secrets (BEFORE aggregator needs them)
# ──────────────────────────────────────────────────────────────
Write-Host "`n--- Setting up NSG Firewall ---" -ForegroundColor Yellow
& "$scriptDir\setup-nsg.ps1" -ResourceGroup $ResourceGroup -AksName $AksName

# Apply manifests with ACR substitution (aggregator now has secrets available)
foreach ($f in @("01-aggregator.yaml", "02-iot-nodes.yaml", "03-normal-traffic.yaml")) {
    Write-Host "Applying $f..."
    (Get-Content "$scriptDir\$f" -Raw) -replace '__ACR__', $acrLoginServer | kubectl apply -f -
}

# Apply monitoring (no ACR substitution needed)
Write-Host "Applying 04-monitoring.yaml..."
kubectl apply -f "$scriptDir\04-monitoring.yaml"

# Create Grafana dashboard ConfigMap from file
Write-Host "Creating Grafana dashboard ConfigMap..."
kubectl create configmap grafana-dashboards `
    --from-file=ddos_dashboard.json="$projectDir\monitoring\grafana\dashboards\ddos_dashboard.json" `
    -n ddos-demo --dry-run=client -o yaml | kubectl apply -f -

# Restart Grafana to pick up dashboard
kubectl rollout restart deployment grafana -n ddos-demo

# ──────────────────────────────────────────────────────────────
# Section 5: Wait for Deployments
# ──────────────────────────────────────────────────────────────
Write-Host "`n--- Waiting for pods ---" -ForegroundColor Yellow
kubectl wait --for=condition=available deployment --all -n ddos-demo --timeout=120s

Write-Host "`n--- Pod Status ---"
kubectl get pods -n ddos-demo -o wide

# ──────────────────────────────────────────────────────────────
# Section 6: Get Grafana URL
# ──────────────────────────────────────────────────────────────
Write-Host "`n--- Getting Grafana URL ---" -ForegroundColor Yellow
Write-Host "Waiting for LoadBalancer IP..."
$attempts = 0
do {
    $grafanaIP = kubectl get svc grafana -n ddos-demo -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>$null
    $attempts++
    if ([string]::IsNullOrEmpty($grafanaIP) -and $attempts -lt 30) {
        Write-Host "  Waiting... ($attempts/30)"
    }
} while ([string]::IsNullOrEmpty($grafanaIP) -and $attempts -lt 30)

Write-Host "`n========================================" -ForegroundColor Green
Write-Host "  Deployment Complete!"                      -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Grafana:  http://${grafanaIP}:3000"        -ForegroundColor White
Write-Host "Login:    admin / admin"                     -ForegroundColor White
Write-Host ""
Write-Host "To trigger attack:"                          -ForegroundColor Yellow
Write-Host "  kubectl apply -f $scriptDir\05-attack.yaml"
Write-Host ""
Write-Host "To re-trigger:"                              -ForegroundColor Yellow
Write-Host "  kubectl delete jobs attack-node-2 attack-node-3 -n ddos-demo"
Write-Host "  kubectl apply -f $scriptDir\05-attack.yaml"
