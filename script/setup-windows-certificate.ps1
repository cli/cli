$scriptPath = split-path -parent $MyInvocation.MyCommand.Definition
$certFile = "$scriptPath\windows-certificate.pfx"

$headers = New-Object "System.Collections.Generic.Dictionary[[String],[String]]"
$headers.Add("Authorization", "token $env:DESKTOP_CERT_TOKEN")
$headers.Add("Accept", 'application/vnd.github.v3.raw')

Invoke-WebRequest 'https://api.github.com/repos/desktop/desktop-secrets/contents/windows-certificate.pfx' `
              -Headers $headers `
              -OutFile "$certFile"

Write-Output "::set-output name=cert-file::$certFile"
