[CmdletBinding()]
param (
    [Parameter(Mandatory=$true, Position=0)]
    [string] $Path,

    [Parameter(Mandatory=$true, Position=1)]
    [string] $OutFile,

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string] $FontFamily = 'Arial'
)

$rtf = "{\rtf1\ansi\deff0{\fonttbl{\f0\fcharset0 $FontFamily;}}\pard\sa200\sl200\slmult1\fs20`n"
foreach ($line in (Get-Content $Path)) {
    if (!$line) {
        $rtf += "\par`n"
    } else {
        $rtf += "$line`n"
    }
}
$rtf += '}'

$rtf | Set-Content $OutFile