# GitHub CLI Integration Plan

## Overview
Integrate custom-compiled GitHub CLI binaries with existing AdGenXAI PR automation system.

## 1. Enhanced CLI Integration

### A. Update PowerShell Automation
```powershell
# Add to pr-automation.ps1 header
$CUSTOM_GH_CLI = "C:\Users\north\gh-cli\bin\gh.exe"

function Test-CustomGitHubCLI {
    try {
        $version = & $CUSTOM_GH_CLI version 2>&1
        Write-Status "Custom GitHub CLI ready: $version" "Success"
        return $true
    } catch {
        Write-Status "Custom GitHub CLI failed: $($_.Exception.Message)" "Error"
        return $false
    }
}

function Get-PRsWithCustomCLI {
    param($State = "open", $Limit = 50)
    
    $prData = & $CUSTOM_GH_CLI pr list --repo $Repo --state $State --limit $Limit --json number,title,author,reviewDecision,mergeable,statusCheckRollup
    return $prData | ConvertFrom-Json
}
```

### B. Enhanced Health Checks
```powershell
# Add to existing health check section
Write-Host "`n[11/11] Testing Custom GitHub CLI integration..."
if (Test-CustomGitHubCLI) {
    try {
        $repoAccess = & $CUSTOM_GH_CLI repo view $Repo --json name,description,defaultBranch
        $repoInfo = $repoAccess | ConvertFrom-Json
        Write-Host "Repository access confirmed: $($repoInfo.name)" -ForegroundColor Green
        
        $prCount = & $CUSTOM_GH_CLI pr list --repo $Repo --state open --limit 1 --json number | ConvertFrom-Json | Measure-Object | Select-Object -ExpandProperty Count
        Write-Host "Open PRs detected: $prCount" -ForegroundColor Green
    } catch {
        Write-Host "GitHub CLI repo access failed: $($_.Exception.Message)" -ForegroundColor Yellow
    }
}
```

## 2. Cross-Platform Deployment

### A. Raspberry Pi ARM Deployment
```bash
#!/bin/bash
# deploy-arm.sh - Deploy to Raspberry Pi
scp bin/gh pi@raspberrypi:/usr/local/bin/gh-custom
ssh pi@raspberrypi "chmod +x /usr/local/bin/gh-custom"
ssh pi@raspberrypi "/usr/local/bin/gh-custom auth login --with-token < ~/.github-token"
```

### B. 24/7 ARM Automation Service
```bash
# raspberry-pi-automation.sh
#!/bin/bash
export GH_CUSTOM="/usr/local/bin/gh-custom"

while true; do
    echo "[$(date)] Running PR health check..."
    
    # Get critical PRs
    CRITICAL_PRS=$($GH_CUSTOM pr list --repo brandonlacoste9-tech/adgenxai --label "critical" --state open --json number,title)
    
    if [ $(echo "$CRITICAL_PRS" | jq length) -gt 0 ]; then
        echo "Critical PRs detected, triggering notification..."
        # Send notification logic here
    fi
    
    sleep 300  # Check every 5 minutes
done
```

## 3. Advanced PR Automation Features

### A. Custom CLI Commands for PR Manager
```javascript
// Add to src/github-cli-integration.js
import { exec } from 'child_process';
import { promisify } from 'util';

const execAsync = promisify(exec);
const CUSTOM_GH_CLI = process.env.CUSTOM_GH_CLI || 'C:\\Users\\north\\gh-cli\\bin\\gh.exe';

export class CustomGitHubCLI {
    async getPRDetails(repo, prNumber) {
        const cmd = `${CUSTOM_GH_CLI} pr view ${prNumber} --repo ${repo} --json number,title,body,author,reviewDecision,statusCheckRollup,mergeable`;
        const { stdout } = await execAsync(cmd);
        return JSON.parse(stdout);
    }
    
    async mergePR(repo, prNumber, mergeMethod = 'squash') {
        const cmd = `${CUSTOM_GH_CLI} pr merge ${prNumber} --repo ${repo} --${mergeMethod} --delete-branch`;
        const { stdout } = await execAsync(cmd);
        return stdout;
    }
    
    async reviewPR(repo, prNumber, action, body) {
        const cmd = `${CUSTOM_GH_CLI} pr review ${prNumber} --repo ${repo} --${action} --body "${body}"`;
        const { stdout } = await execAsync(cmd);
        return stdout;
    }
}
```

### B. Enhanced Metrics Collection
```javascript
// Add to enhanced-metrics.js
export const customCliMetrics = {
    cliOperationsTotal: new client.Counter({
        name: 'github_cli_operations_total',
        help: 'Total custom GitHub CLI operations',
        labelNames: ['operation', 'status']
    }),
    
    cliResponseTime: new client.Histogram({
        name: 'github_cli_response_seconds',
        help: 'Custom GitHub CLI response time',
        labelNames: ['operation']
    })
};

export function recordCLIOperation(operation, status, duration) {
    customCliMetrics.cliOperationsTotal.inc({ operation, status });
    if (duration) {
        customCliMetrics.cliResponseTime.observe({ operation }, duration);
    }
}
```

## 4. Monitoring Integration

### A. Grafana Dashboard Updates
```json
{
  "title": "Custom GitHub CLI Operations",
  "targets": [
    {
      "expr": "rate(github_cli_operations_total[5m])",
      "legendFormat": "{{operation}} - {{status}}"
    },
    {
      "expr": "histogram_quantile(0.95, rate(github_cli_response_seconds_bucket[5m]))",
      "legendFormat": "95th percentile response time"
    }
  ]
}
```

### B. Health Check Enhancement
```javascript
// Add to health-monitor.js
async checkCustomGitHubCLI() {
    try {
        const start = Date.now();
        const { stdout } = await execAsync(`${CUSTOM_GH_CLI} auth status`);
        const duration = Date.now() - start;
        
        recordCLIOperation('auth_check', 'success', duration);
        return { status: 'healthy', version: stdout.trim(), responseTime: duration };
    } catch (error) {
        recordCLIOperation('auth_check', 'error');
        return { status: 'unhealthy', error: error.message };
    }
}
```

## 5. Implementation Priority

### Phase 1 (Immediate) ✅
- [x] Build custom GitHub CLI binaries
- [x] Test Windows binary functionality
- [ ] Update pr-automation.ps1 with custom CLI

### Phase 2 (This Week)
- [ ] Integrate custom CLI into PR Manager health checks
- [ ] Add CLI metrics to enhanced-metrics.js
- [ ] Create ARM deployment script

### Phase 3 (Next Week)
- [ ] Deploy ARM binary to Raspberry Pi
- [ ] Set up 24/7 automation service
- [ ] Enhanced Grafana dashboards

## Benefits

1. **Custom Control**: Full control over GitHub CLI functionality
2. **Cross-Platform**: Same automation logic on Windows + ARM devices
3. **Enhanced Monitoring**: Custom metrics for CLI operations
4. **Resilience**: Custom CLI integrates with existing circuit breakers
5. **24/7 Operations**: ARM deployment for continuous monitoring

## Next Steps

1. Run `Test-CustomGitHubCLI` function integration
2. Deploy enhanced health checks
3. Create ARM deployment package
4. Update monitoring dashboards
