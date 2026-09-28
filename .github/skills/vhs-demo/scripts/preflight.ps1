$ErrorActionPreference = "Stop"

function Invoke-VersionProbe {
    param(
        [Parameter(Mandatory)]
        [string]$Name,
        [Parameter(Mandatory)]
        [string[]]$Arguments
    )

    $command = Get-Command $Name -CommandType Application -ErrorAction SilentlyContinue
    if ($null -eq $command) {
        throw "VHS demo preflight failed: missing required tool: $Name"
    }

    $output = (& $command.Path @Arguments 2>&1 | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) {
        throw "VHS demo preflight failed: $Name exists at $($command.Path) but could not execute"
    }
    if ([string]::IsNullOrWhiteSpace($output)) {
        throw "VHS demo preflight failed: $Name returned no version information"
    }

    $firstLine = ($output -split "\r?\n")[0]
    Write-Host "${Name}: $($command.Path) ($firstLine)"
    return $output
}

$vhsOutput = Invoke-VersionProbe -Name "vhs" -Arguments @("--version")
$match = [regex]::Match($vhsOutput, 'v?(\d+\.\d+(?:\.\d+)?)')
if (-not $match.Success) {
    throw "VHS demo preflight failed: unable to parse VHS version from: $vhsOutput"
}
if ([version]$match.Groups[1].Value -lt [version]"0.11.0") {
    throw "VHS demo preflight failed: VHS 0.11.0 or newer is required; found $($match.Groups[1].Value)"
}

Invoke-VersionProbe -Name "ffmpeg" -Arguments @("-version") | Out-Null
Invoke-VersionProbe -Name "ffprobe" -Arguments @("-version") | Out-Null
Invoke-VersionProbe -Name "ttyd" -Arguments @("--version") | Out-Null

Write-Output "VHS demo prerequisites satisfied."
