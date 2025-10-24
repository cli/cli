# Carpuncle Cloud Security Guide

## 🔐 Sicherheitsübersicht

Dieses Dokument beschreibt die Sicherheitsmaßnahmen und Best Practices für Carpuncle Cloud.

## Branch Protection Rules

### Geschützte Branches

#### Main Branch
- **Branch**: `main`
- **Require pull request reviews**: ✅ Enabled (min. 1 reviewer)
- **Require status checks**: ✅ Enabled
- **Require branches to be up to date**: ✅ Enabled
- **Include administrators**: ✅ Enabled
- **Restrict who can push**: DevOps Team, Release Manager

#### Release Branch
- **Branch**: `release`
- **Require pull request reviews**: ✅ Enabled (min. 2 reviewers)
- **Require status checks**: ✅ Enabled
- **Require signed commits**: ✅ Recommended
- **Include administrators**: ✅ Enabled

### Setup Instructions

```bash
# Via GitHub CLI
gh api repos/Carpuncle-Lana/Carpuncle/branches/main/protection \
  -X PUT \
  -H "Accept: application/vnd.github.v3+json" \
  -F required_pull_request_reviews='{"required_approving_review_count":1}' \
  -F required_status_checks='{"strict":true,"contexts":["validate","security-scan"]}' \
  -F enforce_admins=true
```

---

## GitHub Advanced Security

### Code Scanning (CodeQL)

**Status**: ✅ Aktiv (siehe `.github/workflows/codeql.yml`)

**Unterstützte Sprachen**:
- Go (Haupt-Repository)
- PowerShell (Azure Scripts)
- JavaScript/TypeScript (potenzielle Frontend-Komponenten)

**Scan-Frequenz**:
- Bei jedem Push auf `main`
- Bei Pull Requests
- Wöchentlicher Schedule-Scan

### Secret Scanning

**Status**: ✅ Aktiv (siehe `.github/secret_scanning.yml`)

**Geschützte Secrets**:
- Azure Credentials
- API Keys
- Database Connection Strings
- OAuth Tokens
- Private Keys

**Alert-Handling**:
1. Automatische Benachrichtigung bei Erkennung
2. Secret sofort rotieren
3. Commit-History bereinigen (falls erforderlich)
4. Alert schließen nach Verifizierung

### Dependency Scanning

**Status**: ✅ Aktiv (via Dependabot)

**Konfiguration**: `.github/dependabot.yml`

```yaml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
  - package-ecosystem: "npm"
    directory: "/"
    schedule:
      interval: "weekly"
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
```

---

## SSO & 2FA

### Single Sign-On (SSO)

**Provider**: Azure Active Directory

**Setup**:
1. Navigate to Organization Settings → Authentication security
2. Enable SAML single sign-on
3. Configure Azure AD as IdP
4. Test SSO connection
5. Enforce for all organization members

### Two-Factor Authentication (2FA)

**Requirement**: ✅ Enforced for all organization members

**Unterstützte Methoden**:
- Authenticator Apps (Microsoft Authenticator, Google Authenticator)
- SMS (Backup)
- Security Keys (Thetis FIDO)

**Enforcement**:
```bash
# Via GitHub CLI
gh api orgs/carpunclede/settings \
  -X PATCH \
  -F two_factor_requirement_enabled=true
```

---

## Audit Logging

### Log-Arten

1. **Organization Audit Log**: Alle Organisationsaktivitäten
2. **Repository Audit Log**: Repository-spezifische Events
3. **Security Log**: Sicherheitsrelevante Events

### Monitoring-Frequenz

- **Täglich**: Automatisches Monitoring via Azure Monitor
- **Wöchentlich**: Manuelle Review durch Security Team
- **Monatlich**: Umfassender Security Audit

### Wichtige Events

- Member additions/removals
- Permission changes
- Repository access modifications
- Secret access
- Deployment activities
- Failed login attempts

### Export & Archivierung

```bash
# Export Audit Log
gh api orgs/carpunclede/audit-log \
  -H "Accept: application/vnd.github.v3+json" \
  > audit-log-$(date +%Y-%m-%d).json

# Langzeitarchivierung in Azure Storage
az storage blob upload \
  --account-name carpuncledev \
  --container-name audit-logs \
  --name audit-log-$(date +%Y-%m-%d).json \
  --file audit-log-$(date +%Y-%m-%d).json
```

---

## Azure Security

### Key Vault Secrets

**Best Practices**:
1. Alle sensiblen Daten in Azure Key Vault speichern
2. Managed Identities für Zugriff verwenden
3. Secret-Rotation alle 90 Tage
4. Access Policies minimal halten (Principle of Least Privilege)

**Zugriff auf Secrets**:
```powershell
# In Azure Functions oder Deployment Scripts
$secret = Get-AzKeyVaultSecret -VaultName "carpuncle-prod-kv" -Name "ApiKey"
$secretValue = $secret.SecretValue | ConvertFrom-SecureString -AsPlainText
```

### Network Security

**Firewall Rules**:
- Nur HTTPS Traffic erlauben
- Geo-Restriction für kritische APIs
- Rate Limiting aktiv

**Private Endpoints**:
- Storage Accounts über Private Endpoints
- Function Apps im Virtual Network
- Key Vault nur über Private Link

---

## Incident Response

### Security Incident Workflow

1. **Detection**: Automated alerts via Azure Monitor & GitHub
2. **Triage**: Security Team bewertet Schweregrad
3. **Containment**: Sofortige Maßnahmen zur Schadensbegrenzung
4. **Investigation**: Root Cause Analysis
5. **Remediation**: Behebung der Schwachstelle
6. **Documentation**: Post-Mortem Bericht
7. **Review**: Lessons Learned & Prozessverbesserung

### Kontakte bei Security Incidents

- **Security Lead**: security@carpuncle.eu
- **On-Call**: +49-XXX-XXXXXXX
- **Escalation**: Thomas Heckhoff (carpuncle-pc@live.de)

### Reporting

**Intern**: security@carpuncle.eu  
**Extern**: [Security Advisory](https://github.com/Carpuncle-Lana/Carpuncle/security/advisories)

---

## Compliance & Standards

### Applicable Standards

- **GDPR**: Datenschutz für EU-Bürger
- **ISO 27001**: Information Security Management
- **Azure Security Baseline**: Microsoft Best Practices

### Data Classification

| Level | Beschreibung | Beispiele | Schutzmaßnahmen |
|-------|-------------|-----------|-----------------|
| **Public** | Öffentlich verfügbar | README, Docs | Keine speziellen |
| **Internal** | Nur für Organisation | Code, Designs | SSO, 2FA |
| **Confidential** | Vertraulich | API Keys, Secrets | Key Vault, Encryption |
| **Restricted** | Streng vertraulich | Customer Data | Encryption at rest & in transit, Audit Logging |

---

## Security Checklist

### Setup (Initial)

- [ ] Branch Protection für `main` und `release` aktivieren
- [ ] CodeQL Code Scanning einrichten
- [ ] Secret Scanning aktivieren
- [ ] Dependabot konfigurieren
- [ ] SSO mit Azure AD einrichten
- [ ] 2FA für alle Mitglieder erzwingen
- [ ] Azure Key Vault erstellen
- [ ] Security Team bilden

### Laufender Betrieb (Recurring)

- [ ] Wöchentliche Audit Log Review
- [ ] Monatliche Access Rights Review
- [ ] Quartalsweise Security Audits
- [ ] Secret Rotation (90 Tage)
- [ ] Dependency Updates prüfen
- [ ] Security Training für Team

### Bei neuen Features

- [ ] Threat Modeling durchführen
- [ ] Security Review vor Merge
- [ ] Penetration Testing (bei kritischen Features)
- [ ] Security Documentation aktualisieren

---

## Tools & Resources

### Empfohlene Tools

- **GitHub Advanced Security**: Code & Secret Scanning
- **Azure Security Center**: Cloud Security Posture Management
- **Dependabot**: Dependency Management
- **CodeQL**: Static Application Security Testing
- **Azure Key Vault**: Secret Management
- **Microsoft Defender**: Threat Protection

### Weiterführende Dokumentation

- [GitHub Security Best Practices](https://docs.github.com/en/code-security)
- [Azure Security Documentation](https://learn.microsoft.com/en-us/azure/security/)
- [OWASP Top 10](https://owasp.org/www-project-top-ten/)

---

*Letzte Aktualisierung: 2024-10-24*  
*Version: 1.0.0*  
*Security Team: security@carpuncle.eu*
