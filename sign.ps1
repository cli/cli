param (
    [string]$Certificate = $(throw "-Certificate is required."),
    [string]$Executable = $(throw "-Executable is required.")
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$thumbprint = "fb713a60a7fa79dfc03cb301ca05d4e8c1bdd431"
$passwd = $env:GITHUB_CERT_PASSWORD
$ProgramName = "GitHub CLI"

& .\signtool.exe sign /d $ProgramName /f $Certificate /p $passwd /sha1 $thumbprint /fd sha256 /tr http://timestamp.digicert.com /td sha256 /v $Executable
