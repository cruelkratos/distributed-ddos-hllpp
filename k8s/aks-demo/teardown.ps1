# teardown.ps1 — Clean up AKS deployment
param(
    [string]$ResourceGroup = "ddos-demo-rg",
    [string]$Namespace = "ddos-demo",
    [switch]$DeleteAll
)

$ErrorActionPreference = "Stop"

Write-Host "=== DDoS Detection - Teardown ===" -ForegroundColor Cyan

# Delete K8s namespace (removes all resources)
Write-Host "Deleting namespace '$Namespace'..."
kubectl delete namespace $Namespace --ignore-not-found

# Delete the NSG service principal
Write-Host "Cleaning up NSG service principal..."
$spId = az ad sp list --display-name "ddos-nsg-controller" --query "[0].id" -o tsv 2>$null
if (-not [string]::IsNullOrEmpty($spId)) {
    az ad sp delete --id $spId
    Write-Host "  Service principal deleted"
}

if ($DeleteAll) {
    Write-Host "`nDeleting entire resource group '$ResourceGroup'..." -ForegroundColor Red
    Write-Host "  This will delete AKS cluster, ACR, NSG, and all resources."
    $confirm = Read-Host "  Type 'yes' to confirm"
    if ($confirm -eq "yes") {
        az group delete -n $ResourceGroup --yes --no-wait
        Write-Host "  Resource group deletion initiated (runs in background)"
    } else {
        Write-Host "  Cancelled."
    }
} else {
    Write-Host "`nNamespace deleted. Azure resources (AKS, ACR) still running."
    Write-Host "To delete everything: .\teardown.ps1 -DeleteAll" -ForegroundColor Yellow
}

Write-Host "`n=== Teardown Complete ===" -ForegroundColor Green
