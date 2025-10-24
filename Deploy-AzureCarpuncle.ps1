# Deploy-AzureCarpuncle.ps1
# Automatisiertes Azure-Deployment für Carpuncle Cloud
# Phase 1-4: Infrastruktur, Tools, Initialisierung, Integration

<#
.SYNOPSIS
    Automatisiertes Deployment-Skript für Carpuncle Cloud auf Azure

.DESCRIPTION
    Dieses Skript richtet die komplette Carpuncle Cloud Infrastruktur auf Azure ein:
    - Ressourcengruppen
    - Storage Accounts
    - Function Apps
    - Key Vault
    - Monitoring

.PARAMETER ResourceGroupName
    Name der Azure Ressourcengruppe (Standard: carpuncle-cloud-rg)

.PARAMETER Location
    Azure Region (Standard: westeurope)

.PARAMETER Environment
    Deployment-Umgebung: dev, test, prod (Standard: dev)

.EXAMPLE
    .\Deploy-AzureCarpuncle.ps1 -Environment prod -Location westeurope
#>

param(
    [Parameter(Mandatory=$false)]
    [string]$ResourceGroupName = "carpuncle-cloud-rg",
    
    [Parameter(Mandatory=$false)]
    [string]$Location = "westeurope",
    
    [Parameter(Mandatory=$false)]
    [ValidateSet("dev", "test", "prod")]
    [string]$Environment = "dev",
    
    [Parameter(Mandatory=$false)]
    [string]$SubscriptionId
)

# Farben für Console Output
$InfoColor = "Cyan"
$SuccessColor = "Green"
$WarningColor = "Yellow"
$ErrorColor = "Red"

function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor $InfoColor
}

function Write-Success {
    param([string]$Message)
    Write-Host "[SUCCESS] $Message" -ForegroundColor $SuccessColor
}

function Write-Warning {
    param([string]$Message)
    Write-Host "[WARNING] $Message" -ForegroundColor $WarningColor
}

function Write-Error {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor $ErrorColor
}

# Hauptskript
try {
    Write-Info "=== Carpuncle Cloud Azure Deployment ==="
    Write-Info "Environment: $Environment"
    Write-Info "Location: $Location"
    Write-Info "Resource Group: $ResourceGroupName"
    
    # 1. Azure Login prüfen
    Write-Info "Prüfe Azure-Anmeldung..."
    $context = Get-AzContext
    
    if (-not $context) {
        Write-Warning "Nicht bei Azure angemeldet. Führe Login durch..."
        Connect-AzAccount
    }
    
    # Subscription setzen falls angegeben
    if ($SubscriptionId) {
        Write-Info "Setze Subscription: $SubscriptionId"
        Set-AzContext -SubscriptionId $SubscriptionId
    }
    
    $context = Get-AzContext
    Write-Success "Angemeldet als: $($context.Account.Id)"
    Write-Success "Subscription: $($context.Subscription.Name)"
    
    # 2. Ressourcengruppe erstellen oder prüfen
    Write-Info "Prüfe Ressourcengruppe..."
    $rg = Get-AzResourceGroup -Name $ResourceGroupName -ErrorAction SilentlyContinue
    
    if (-not $rg) {
        Write-Info "Erstelle Ressourcengruppe: $ResourceGroupName"
        $rg = New-AzResourceGroup -Name $ResourceGroupName -Location $Location -Tag @{
            Environment = $Environment
            Project = "Carpuncle-Cloud"
            ManagedBy = "Deploy-AzureCarpuncle.ps1"
        }
        Write-Success "Ressourcengruppe erstellt"
    } else {
        Write-Success "Ressourcengruppe existiert bereits"
    }
    
    # 3. Storage Account für Carpuncle Daten
    Write-Info "Prüfe Storage Account..."
    $storageAccountName = "carpuncle$Environment".ToLower() -replace '[^a-z0-9]', ''
    
    # Storage Account Name muss 3-24 Zeichen haben
    if ($storageAccountName.Length -gt 24) {
        $storageAccountName = $storageAccountName.Substring(0, 24)
    }
    
    $storage = Get-AzStorageAccount -ResourceGroupName $ResourceGroupName -Name $storageAccountName -ErrorAction SilentlyContinue
    
    if (-not $storage) {
        Write-Info "Erstelle Storage Account: $storageAccountName"
        $storage = New-AzStorageAccount `
            -ResourceGroupName $ResourceGroupName `
            -Name $storageAccountName `
            -Location $Location `
            -SkuName Standard_LRS `
            -Kind StorageV2 `
            -Tag @{
                Environment = $Environment
                Project = "Carpuncle-Cloud"
            }
        Write-Success "Storage Account erstellt"
    } else {
        Write-Success "Storage Account existiert bereits"
    }
    
    # 4. Key Vault für Secrets
    Write-Info "Prüfe Key Vault..."
    $keyVaultName = "carpuncle-$Environment-kv"
    
    $keyVault = Get-AzKeyVault -VaultName $keyVaultName -ResourceGroupName $ResourceGroupName -ErrorAction SilentlyContinue
    
    if (-not $keyVault) {
        Write-Info "Erstelle Key Vault: $keyVaultName"
        $keyVault = New-AzKeyVault `
            -VaultName $keyVaultName `
            -ResourceGroupName $ResourceGroupName `
            -Location $Location `
            -Tag @{
                Environment = $Environment
                Project = "Carpuncle-Cloud"
            }
        Write-Success "Key Vault erstellt"
    } else {
        Write-Success "Key Vault existiert bereits"
    }
    
    # 5. Function App für Lana KI-Module
    Write-Info "Prüfe Function App..."
    $functionAppName = "carpuncle-lana-$Environment"
    
    $functionApp = Get-AzFunctionApp -ResourceGroupName $ResourceGroupName -Name $functionAppName -ErrorAction SilentlyContinue
    
    if (-not $functionApp) {
        Write-Info "Erstelle Function App: $functionAppName"
        
        # App Service Plan erstellen
        $appServicePlanName = "carpuncle-plan-$Environment"
        $appServicePlan = Get-AzAppServicePlan -ResourceGroupName $ResourceGroupName -Name $appServicePlanName -ErrorAction SilentlyContinue
        
        if (-not $appServicePlan) {
            Write-Info "Erstelle App Service Plan: $appServicePlanName"
            $appServicePlan = New-AzAppServicePlan `
                -ResourceGroupName $ResourceGroupName `
                -Name $appServicePlanName `
                -Location $Location `
                -Tier "Dynamic" `
                -WorkerSize "Small"
            Write-Success "App Service Plan erstellt"
        }
        
        # Function App erstellen
        $functionApp = New-AzFunctionApp `
            -ResourceGroupName $ResourceGroupName `
            -Name $functionAppName `
            -StorageAccountName $storageAccountName `
            -Location $Location `
            -Runtime PowerShell `
            -RuntimeVersion "7.2" `
            -FunctionsVersion 4 `
            -Tag @{
                Environment = $Environment
                Project = "Carpuncle-Cloud"
                Component = "Lana-KI"
            }
        Write-Success "Function App erstellt"
    } else {
        Write-Success "Function App existiert bereits"
    }
    
    # 6. Application Insights für Monitoring
    Write-Info "Prüfe Application Insights..."
    $appInsightsName = "carpuncle-insights-$Environment"
    
    # Application Insights mit Az.ApplicationInsights Modul
    $appInsights = Get-AzApplicationInsights -ResourceGroupName $ResourceGroupName -Name $appInsightsName -ErrorAction SilentlyContinue
    
    if (-not $appInsights) {
        Write-Info "Erstelle Application Insights: $appInsightsName"
        $appInsights = New-AzApplicationInsights `
            -ResourceGroupName $ResourceGroupName `
            -Name $appInsightsName `
            -Location $Location `
            -Tag @{
                Environment = $Environment
                Project = "Carpuncle-Cloud"
            }
        Write-Success "Application Insights erstellt"
    } else {
        Write-Success "Application Insights existiert bereits"
    }
    
    # 7. Zusammenfassung
    Write-Info ""
    Write-Info "=== Deployment Zusammenfassung ==="
    Write-Success "✓ Ressourcengruppe: $ResourceGroupName"
    Write-Success "✓ Storage Account: $storageAccountName"
    Write-Success "✓ Key Vault: $keyVaultName"
    Write-Success "✓ Function App: $functionAppName"
    Write-Success "✓ Application Insights: $appInsightsName"
    Write-Info ""
    Write-Success "=== Deployment erfolgreich abgeschlossen ==="
    Write-Info ""
    Write-Info "Nächste Schritte:"
    Write-Info "1. Key Vault Secrets konfigurieren"
    Write-Info "2. Function App Code deployen"
    Write-Info "3. Storage Container für Daten erstellen"
    Write-Info "4. Monitoring Dashboard einrichten"
    
} catch {
    Write-Error "Deployment fehlgeschlagen: $_"
    Write-Error $_.Exception.Message
    exit 1
}
