# Carpuncle Cloud - GitHub Copilot Instructions

## Projektübersicht

**Carpuncle Cloud** ist ein Azure-basiertes KI-Framework für automatisierte Cloud-Workflows, Webhooks und Datenanalyse mit integriertem Dashboard und Backup-Funktionen.

### Kernkomponenten

- **Carpuncle-Cloud**: Azure-basierte Infrastruktur mit Dashboard
- **lana-core**: KI-Module für Lana (selbstlernende KI-Assistenz)
- **dotnet**: Backend-Services in .NET
- **Automatisierungsketten**: 8 Hauptautomatisierungen (Start, Scan, Manifest, Korrektur, Dashboard, Sicherheit, Dokumentation, SharePoint)

## Technologien

### Haupttechnologien

- **Cloud & Infrastructure**: Azure CLI, PowerShell 7
- **Backend**: Node.js 18, .NET, Go
- **CI/CD**: GitHub Actions
- **APIs**: Microsoft Graph API
- **Sicherheit**: Thetis FIDO, RoboForm, 2FA/SSO

### Entwickler-Tools & Package Manager

**Phase 2 - Developer Engine & Tool Recognition**:
- **Programmiersprachen**: Python, C#, C++, Java, JavaScript, Go, Rust
- **Package Manager**: npm, pip, cargo, brew, conda, dotnet, nym, winger
- **Wichtig**: Tools lokal im Projektordner installieren, keine Systemabhängigkeiten

## Build & Deployment

### Deployment-Skript

```powershell
# Hauptskript für Azure-Deployment
.\Deploy-AzureCarpuncle.ps1
```

### Zielumgebung

- **Azure**: Testversion / Enterprise-Subscription
- **Branch Strategy**: 
  - `main`: Geschützt, für Production
  - `release`: Geschützt, für Releases
  - Feature-Branches für Entwicklung

### CI/CD Workflow

- **Trigger**: Push auf `main` Branch
- **Schritte**: Azure Login → Ressourcen prüfen → Skript ausführen
- **Secrets**: `AZURE_CREDENTIALS`, `SUBSCRIPTION_ID`, `APP_ID`

## Projektstruktur

### Verzeichnisstruktur (Phase 5)

```
T:\Carpuncle\
├── Enrw/           # Entwicklung & Produktion
├── Projekte/       # Projektdateien
├── Logs/           # Log-Dateien
└── Alliase/        # Alias-Konfigurationen
```

### Repository-Organisation

| Repository | Zweck |
|-----------|-------|
| `Carpuncle-Cloud` | Hauptprojekt mit Azure-Setup |
| `dotnet` | Backend-Module und APIs |
| `lana-core` | KI-Logik & Lana-Funktionen |
| `carpuncle-docs` | Dokumentation & Playbooks |

## Coding Standards

### Allgemein

- Minimale Änderungen bevorzugen
- Bestehende Konventionen respektieren
- Keine Kommentare hinzufügen, es sei denn notwendig
- Bestehende Libraries verwenden

### Sicherheit (Phase 4 & 6)

- **SSL/OAuth**: Immer aktiviert
- **Zugriffskontrolle**: Identity-basierte Steuerung
- **Code Scanning**: GitHub Advanced Security aktiv
- **Branch Protection**: Für `main` und `release` Branches
- **Audit Logs**: Regelmäßige Überprüfung

### Testing & Qualität (Phase 3)

- **Build & Run**: Erfolg-Integration erforderlich
- **Fehlerbehandlung**: Alternative Lösungen bereitstellen
- **SDK**: Wechsel und Neuinitialisierung bei Bedarf
- **Alternative Compiler**: Optional verfügbar

## Organisationsstruktur

### GitHub Enterprise Setup

- **Enterprise**: `carpuncle`
- **Organisation**: `carpunclede` (Technische Repos)
- **Benutzer**: `Carpuncle-Lana` (Persönliches Profil)
- **Teams**: DevOps, KI, Frontend, Security

### Labels & Projects

Verwende folgende Labels für Issues und PRs:
- `feature`: Neue Funktionalität
- `bug`: Fehlerbehebungen
- `infra`: Infrastruktur-Änderungen
- `ki`: KI/ML-bezogene Änderungen
- `dashboard`: Dashboard-Funktionalität

## Phase-Spezifische Hinweise

### Phase 1: Identität & Infrastruktur
- OneDrive & Google Drive Integration
- Geräteunabhängiges Geräte-Syncri
- Alias-Verwaltung: `carpuncle-pc@live.de`, `thoma@carpuncle.eu`, `carpV@carpuncle.eu`

### Phase 2: Entwickler-Engine
- Multi-Language Support
- Lokale Tool-Installation
- Keine Systemabhängigkeiten

### Phase 3: Projektinitialisierung
- Framework-basierte Initialisierung (z.B. `carpuncle init`)
- Automatisierte Tests
- Build & Run Integration

### Phase 4: Webhook & Framework-Integration
- `.carpuncle.eu` als zentrale API-Schnittstelle
- AI-Login und Datenanalyse über zentrale Plattform
- SSL & OAuth durchgehend aktiv

### Phase 5: Speicher & Dokumentation
- Google Drive für persönliche Daten: `CARPUNCLE/`
- Strukturierte Verschlüsselung
- Optional: Geräte-Cloud-Synchronisation

### Phase 6: Erweiterung & KI-Assistenz
- Fehlererkennung mit automatischer Lösungssuche
- Toot, Code, Struktur-Sicherheitsvorschläge
- Identity-basierte Zugriffskontrolle

## Wichtige Kontakte

- **Inhaber**: Thomas Heckhoff
- **Email**: `carpuncle-pc@live.de`
- **GitHub**: [@Carpuncle-Lana](https://github.com/Carpuncle-Lana)
- **Organisation**: [carpunclede](https://github.com/carpunclede)

## Monitoring & Insights

### GitHub Enterprise Insights
- Commit-Aktivität überwachen
- Issue-Flow analysieren
- Deployment-Frequenz tracken

### Status Badges
README sollte folgende Badges enthalten:
- Build Status
- Test Coverage
- Security Scanning
- Deployment Status

## Hilfreiche Befehle

```bash
# Repository klonen
gh repo clone Carpuncle-Lana/Carpuncle

# Azure Login
az login

# PowerShell Profil mit Lana laden
. $PROFILE

# Tests ausführen
npm test
dotnet test
go test ./...
```
