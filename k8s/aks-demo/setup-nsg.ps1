# setup-nsg.ps1 — Create Azure NSG and K8s secrets for firewall integration
# Run AFTER AKS cluster is created and kubectl is configured.
param(
    [Parameter(Mandatory=$true)][string]$ResourceGroup,
    [Parameter(Mandatory=$true)][string]$AksName,
    [string]$NsgName = "ddos-demo-nsg",
    [string]$Location = "eastus"
)

$ErrorActionPreference = "Stop"

Write-Host "`n=== Azure NSG Setup ===" -ForegroundColor Cyan

# 1. Get AKS node resource group and VNet/Subnet info
Write-Host "Getting AKS node resource group..."
$nodeRG = az aks show -g $ResourceGroup -n $AksName --query nodeResourceGroup -o tsv
Write-Host "  Node RG: $nodeRG"

Write-Host "Getting AKS subnet..."
$subnetId = az aks show -g $ResourceGroup -n $AksName `
    --query "agentPoolProfiles[0].vnetSubnetId" -o tsv

if ([string]::IsNullOrEmpty($subnetId)) {
    # Default VNet — find it in node resource group
    $vnetName = az network vnet list -g $nodeRG --query "[0].name" -o tsv
    $subnetName = az network vnet subnet list -g $nodeRG --vnet-name $vnetName --query "[0].name" -o tsv
    $subnetId = az network vnet subnet show -g $nodeRG --vnet-name $vnetName -n $subnetName --query id -o tsv
}
Write-Host "  Subnet: $subnetId"

# 2. Create NSG
Write-Host "`nCreating NSG '$NsgName'..."
az network nsg create -g $nodeRG -n $NsgName -l $Location -o none
Write-Host "  NSG created: $NsgName"

# 3. Add default allow rules (AKS internal traffic)
Write-Host "Adding default allow rules..."
az network nsg rule create -g $nodeRG --nsg-name $NsgName `
    -n AllowAKSInternal --priority 200 --direction Inbound `
    --access Allow --protocol "*" `
    --source-address-prefixes VirtualNetwork `
    --destination-address-prefixes "*" `
    --destination-port-ranges "*" -o none

az network nsg rule create -g $nodeRG --nsg-name $NsgName `
    -n AllowLoadBalancer --priority 300 --direction Inbound `
    --access Allow --protocol "*" `
    --source-address-prefixes AzureLoadBalancer `
    --destination-address-prefixes "*" `
    --destination-port-ranges "*" -o none

az network nsg rule create -g $nodeRG --nsg-name $NsgName `
    -n AllowGrafana --priority 150 --direction Inbound `
    --access Allow --protocol Tcp `
    --source-address-prefixes Internet `
    --destination-address-prefixes "*" `
    --destination-port-ranges 3000 -o none

# 4. Attach NSG to subnet
Write-Host "Attaching NSG to AKS subnet..."
# Extract VNet info from subnet ID
$subnetParts = $subnetId -split "/"
$subnetRG = $subnetParts[4]
$subnetVNet = $subnetParts[8]
$subnetSubnet = $subnetParts[10]
$nsgId = az network nsg show -g $nodeRG -n $NsgName --query id -o tsv

az network vnet subnet update -g $subnetRG --vnet-name $subnetVNet -n $subnetSubnet `
    --network-security-group $nsgId -o none
Write-Host "  NSG attached to subnet"

# 5. Create service principal for NSG API access
Write-Host "`nCreating service principal for NSG access..."
$subscriptionId = az account show --query id -o tsv
$spJson = az ad sp create-for-rbac `
    --name "ddos-nsg-controller" `
    --role "Network Contributor" `
    --scopes "/subscriptions/$subscriptionId/resourceGroups/$nodeRG" `
    -o json | ConvertFrom-Json

Write-Host "  SP created: $($spJson.appId)"

# 6. Create K8s secrets
Write-Host "`nCreating K8s secrets..."
kubectl create secret generic azure-nsg-config `
    --from-literal=subscription-id=$subscriptionId `
    --from-literal=resource-group=$nodeRG `
    --from-literal=nsg-name=$NsgName `
    -n ddos-demo --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic azure-credentials `
    --from-literal=client-id=$($spJson.appId) `
    --from-literal=client-secret=$($spJson.password) `
    --from-literal=tenant-id=$($spJson.tenant) `
    -n ddos-demo --dry-run=client -o yaml | kubectl apply -f -

Write-Host "`n=== NSG Setup Complete ===" -ForegroundColor Green
Write-Host "NSG: $NsgName in $nodeRG"
Write-Host "K8s secrets created: azure-nsg-config, azure-credentials"
Write-Host "The aggregator will call Azure NSG API to toggle lockdown rules."
