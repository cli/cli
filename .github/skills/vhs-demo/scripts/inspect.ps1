param(
    [Parameter(Mandatory, Position = 0)]
    [string]$Gif,
    [Parameter(Mandatory, Position = 1)]
    [string]$InspectionDirectory
)

$ErrorActionPreference = "Stop"

$gifItem = Get-Item -LiteralPath $Gif -ErrorAction Stop
if ($gifItem.Length -eq 0) {
    throw "GIF is empty: $Gif"
}
if (($gifItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
    throw "GIF must not be a symbolic link or reparse point: $Gif"
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..\..")).Path
if (Test-Path -LiteralPath $InspectionDirectory) {
    throw "Inspection directory already exists: $InspectionDirectory"
}
$inspectionLeaf = Split-Path -Leaf $InspectionDirectory
$inspectionParentInput = Split-Path -Parent $InspectionDirectory
if ([string]::IsNullOrEmpty($inspectionParentInput)) {
    $inspectionParentInput = "."
}
$inspectionParent = (
    Resolve-Path -LiteralPath $inspectionParentInput -ErrorAction Stop
).Path
$inspectionPath = Join-Path $inspectionParent $inspectionLeaf
$comparison = if (
    [Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT
) {
    [StringComparison]::OrdinalIgnoreCase
} else {
    [StringComparison]::Ordinal
}
$repoPrefix = $repoRoot.TrimEnd(
    [IO.Path]::DirectorySeparatorChar,
    [IO.Path]::AltDirectorySeparatorChar
) + [IO.Path]::DirectorySeparatorChar

foreach ($candidate in @($gifItem.FullName, $inspectionPath)) {
    if ($candidate.Equals($repoRoot, $comparison) -or
        $candidate.StartsWith($repoPrefix, $comparison)) {
        throw "Generated media must be outside the repository: $candidate"
    }
}

New-Item -ItemType Directory -Path $inspectionPath | Out-Null

$probe = & ffprobe -v error -select_streams v:0 `
    -show_entries "stream=codec_name,width,height,nb_frames:format=format_name,duration,size" `
    -of "default=noprint_wrappers=1" $gifItem.FullName
if ($LASTEXITCODE -ne 0) {
    throw "ffprobe failed for: $Gif"
}
$probeText = $probe -join [Environment]::NewLine
Write-Output $probeText
if ($probe -notcontains "codec_name=gif") {
    throw "Expected GIF video codec: $Gif"
}
if ($probe -notcontains "format_name=gif") {
    throw "Expected GIF container: $Gif"
}

$sampled = Join-Path $inspectionPath "sampled"
New-Item -ItemType Directory -Force -Path $sampled | Out-Null

& ffmpeg -v error -y -i $gifItem.FullName -frames:v 1 `
    (Join-Path $inspectionPath "first.png")
if ($LASTEXITCODE -ne 0) { throw "Failed to extract first frame" }

& ffmpeg -v error -y -sseof -0.1 -i $gifItem.FullName -frames:v 1 `
    (Join-Path $inspectionPath "final.png")
if ($LASTEXITCODE -ne 0) { throw "Failed to extract final frame" }

& ffmpeg -v error -y -i $gifItem.FullName -vf "fps=1" `
    (Join-Path $sampled "frame-%04d.png")
if ($LASTEXITCODE -ne 0) { throw "Failed to extract sampled frames" }

$all = Join-Path $inspectionPath "all"
New-Item -ItemType Directory -Force -Path $all | Out-Null
& ffmpeg -v error -y -i $gifItem.FullName `
    (Join-Path $all "frame-%06d.png")
if ($LASTEXITCODE -ne 0) { throw "Failed to extract all frames" }

Write-Output "Inspection frames: $inspectionPath"
