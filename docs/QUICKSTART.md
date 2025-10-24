# Carpuncle Cloud - Quick Start Guide

## 🚀 Schnellstart für Entwickler

Dieses Dokument hilft dir, schnell mit Carpuncle Cloud zu starten.

## Voraussetzungen

### Erforderliche Tools

- **Git**: Version 2.x oder höher
- **PowerShell**: Version 7.x oder höher
- **Azure CLI**: Latest version
- **Node.js**: Version 18.x LTS
- **.NET SDK**: Version 6.0 oder höher (optional, für dotnet-Projekte)
- **Go**: Version 1.20+ (optional, für Go-Projekte)

### Konten

- **GitHub Account**: Mit Zugriff auf `Carpuncle-Lana/Carpuncle`
- **Azure Account**: Für Cloud-Deployments
- **Microsoft 365**: Optional, für Graph API Integration

## Installation

### 1. Repository klonen

```bash
# Via HTTPS
git clone https://github.com/Carpuncle-Lana/Carpuncle.git
cd Carpuncle

# Via GitHub CLI (empfohlen)
gh repo clone Carpuncle-Lana/Carpuncle
cd Carpuncle
```

### 2. Azure CLI Setup

```bash
# Azure CLI installieren (falls noch nicht vorhanden)
# macOS
brew install azure-cli

# Windows
winget install Microsoft.AzureCLI

# Linux
curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash

# Login
az login
```

### 3. PowerShell Module installieren

```powershell
# Azure PowerShell Modules
Install-Module -Name Az -AllowClobber -Scope CurrentUser

# Spezifische Module für Carpuncle
Install-Module -Name Az.Resources -Force
Install-Module -Name Az.Storage -Force
Install-Module -Name Az.KeyVault -Force
Install-Module -Name Az.Functions -Force
Install-Module -Name Az.ApplicationInsights -Force
```

## Entwicklungsumgebung

### GitHub Copilot aktivieren

1. Stelle sicher, dass GitHub Copilot in deinem Account aktiviert ist
2. Die Copilot-Konfiguration liegt in `.github/copilot-instructions.md`
3. Copilot wird automatisch projektspezifische Vorschläge machen

### VS Code Extensions (empfohlen)

```bash
# PowerShell Extension
code --install-extension ms-vscode.powershell

# Azure Tools
code --install-extension ms-azuretools.vscode-azurefunctions
code --install-extension ms-azuretools.vscode-azureresourcegroups

# GitHub Extensions
code --install-extension github.copilot
code --install-extension github.vscode-pull-request-github

# YAML Support
code --install-extension redhat.vscode-yaml
```

## Erste Schritte

### 1. Azure Ressourcen deployen

```powershell
# Development Environment
.\Deploy-AzureCarpuncle.ps1 -Environment dev -Location westeurope

# Mit spezifischer Subscription
.\Deploy-AzureCarpuncle.ps1 `
    -Environment dev `
    -Location westeurope `
    -SubscriptionId "your-subscription-id"
```

### 2. Secrets konfigurieren

```bash
# GitHub Secrets (via CLI)
gh secret set AZURE_CREDENTIALS --body "$(az ad sp create-for-rbac --json)"
gh secret set SUBSCRIPTION_ID --body "your-subscription-id"
gh secret set APP_ID --body "your-app-id"
```

### 3. Lokale Entwicklung starten

```bash
# Node.js Projekt (falls vorhanden)
npm install
npm run dev

# .NET Projekt (falls vorhanden)
dotnet restore
dotnet build
dotnet run

# Go Projekt
go mod download
go build
./carpuncle
```

## Projekt-Struktur verstehen

```
Carpuncle/
├── .github/                    # GitHub-spezifische Konfiguration
│   ├── copilot-instructions.md # Copilot Anweisungen
│   └── workflows/              # CI/CD Pipelines
│       └── azure-deploy.yml    # Azure Deployment
├── docs/                       # Dokumentation
│   ├── IMPLEMENTATION_PLAN.md  # 6-Phasen-Plan
│   ├── SECURITY_GUIDE.md       # Sicherheitsrichtlinien
│   └── QUICKSTART.md           # Dieses Dokument
├── Deploy-AzureCarpuncle.ps1   # Deployment-Skript
├── src/                        # Source Code (wenn vorhanden)
├── tests/                      # Tests (wenn vorhanden)
└── README.md                   # Projekt-Übersicht
```

## Workflows

### Feature Development

```bash
# 1. Branch erstellen
git checkout -b feature/mein-feature

# 2. Änderungen machen
# ... code changes ...

# 3. Testen
npm test  # oder dotnet test, go test, etc.

# 4. Commit & Push
git add .
git commit -m "feat: Beschreibung der Änderung"
git push origin feature/mein-feature

# 5. Pull Request erstellen
gh pr create --title "Mein Feature" --body "Beschreibung"
```

### Bugfix

```bash
# 1. Branch erstellen
git checkout -b fix/bug-beschreibung

# 2. Fix implementieren
# ... code changes ...

# 3. Testen
npm test

# 4. Commit mit Conventional Commits
git commit -m "fix: Behebe Problem mit XYZ"

# 5. PR erstellen
gh pr create --title "Fix: Problem XYZ" --body "Fixes #123"
```

## Häufige Aufgaben

### Azure Ressourcen prüfen

```powershell
# Ressourcengruppe anzeigen
Get-AzResourceGroup -Name "carpuncle-cloud-rg"

# Alle Ressourcen auflisten
Get-AzResource -ResourceGroupName "carpuncle-cloud-rg" | Format-Table

# Storage Account prüfen
Get-AzStorageAccount -ResourceGroupName "carpuncle-cloud-rg"

# Function App Status
Get-AzFunctionApp -ResourceGroupName "carpuncle-cloud-rg"
```

### Logs ansehen

```bash
# GitHub Actions Logs
gh run list
gh run view <run-id> --log

# Azure Function Logs (via CLI)
az functionapp log tail --name carpuncle-lana-dev --resource-group carpuncle-cloud-rg

# Application Insights
az monitor app-insights query \
  --app carpuncle-insights-dev \
  --analytics-query "traces | take 50"
```

### Secrets verwalten

```powershell
# Key Vault Secret setzen
Set-AzKeyVaultSecret `
  -VaultName "carpuncle-dev-kv" `
  -Name "MySecret" `
  -SecretValue (ConvertTo-SecureString "MySecretValue" -AsPlainText -Force)

# Secret abrufen
$secret = Get-AzKeyVaultSecret -VaultName "carpuncle-dev-kv" -Name "MySecret"
$secretValue = $secret.SecretValue | ConvertFrom-SecureString -AsPlainText
```

## Troubleshooting

### Problem: Azure Login fehlgeschlagen

```bash
# Lösung: Cache löschen und neu anmelden
az account clear
az login
```

### Problem: PowerShell Module nicht gefunden

```powershell
# Lösung: Module neu installieren
Install-Module -Name Az -Force -AllowClobber -Scope CurrentUser
Import-Module Az
```

### Problem: GitHub Actions Workflow schlägt fehl

```bash
# 1. Logs prüfen
gh run list --workflow azure-deploy.yml
gh run view <run-id> --log

# 2. Secrets prüfen
gh secret list

# 3. Secrets neu setzen (falls erforderlich)
gh secret set AZURE_CREDENTIALS --body "$(az ad sp create-for-rbac --json)"
```

### Problem: Deployment-Skript schlägt fehl

```powershell
# 1. Subscription prüfen
Get-AzContext

# 2. Subscription wechseln
Set-AzContext -SubscriptionId "your-subscription-id"

# 3. Berechtigungen prüfen
Get-AzRoleAssignment -SignInName "your-email@example.com"

# 4. Verbose Output aktivieren
.\Deploy-AzureCarpuncle.ps1 -Verbose -Debug
```

## Best Practices

### Commit Messages

Verwende [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: Neue Funktion hinzugefügt
fix: Bug behoben
docs: Dokumentation aktualisiert
style: Code-Formatierung
refactor: Code-Refactoring
test: Tests hinzugefügt
chore: Build-Prozess aktualisiert
```

### Branch Naming

```
feature/kurze-beschreibung
fix/bug-nummer-beschreibung
docs/dokumentation-update
refactor/komponente-name
```

### Code Reviews

- Mindestens 1 Reviewer vor Merge
- Alle Tests müssen grün sein
- Code-Scanning Alerts müssen adressiert werden
- Dokumentation bei Bedarf aktualisieren

### Sicherheit

- **Niemals** Secrets in Code committen
- Verwende Azure Key Vault für sensible Daten
- Aktiviere 2FA für deinen GitHub Account
- Regelmäßige Dependency Updates

## Nützliche Links

### Dokumentation
- [Implementierungsplan](IMPLEMENTATION_PLAN.md)
- [Sicherheitsrichtlinien](SECURITY_GUIDE.md)
- [GitHub Copilot Instructions](../.github/copilot-instructions.md)

### Externe Ressourcen
- [Azure CLI Dokumentation](https://learn.microsoft.com/en-us/cli/azure/)
- [PowerShell Dokumentation](https://learn.microsoft.com/en-us/powershell/)
- [GitHub Actions Dokumentation](https://docs.github.com/en/actions)
- [GitHub CLI Dokumentation](https://cli.github.com/manual/)

### Support
- **Issues**: [GitHub Issues](https://github.com/Carpuncle-Lana/Carpuncle/issues)
- **Discussions**: [GitHub Discussions](https://github.com/Carpuncle-Lana/Carpuncle/discussions)
- **Email**: carpuncle-pc@live.de

## Nächste Schritte

Nach dem Setup kannst du:

1. ✅ Die [Implementierungsphasen](IMPLEMENTATION_PLAN.md) durchgehen
2. ✅ Ein erstes Feature entwickeln
3. ✅ Dich mit dem Team austauschen
4. ✅ Die Dokumentation erweitern
5. ✅ Sicherheitsrichtlinien studieren

---

**Viel Erfolg mit Carpuncle Cloud! 🚀**

*Bei Fragen wende dich an das Team oder erstelle ein Issue.*

---

*Letzte Aktualisierung: 2024-10-24*  
*Version: 1.0.0*
