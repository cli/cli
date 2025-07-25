#!/usr/bin/env pwsh

$_usage = @"
Generate Windows resource files as ``.syso``

Usage:
  gen-winres.ps1 <file-version> <product-version> <path-to-versioninfo.json> <output-path>

Arguments:
  <file-version>              string to set as file version (e.g. "1.0.0")
  <product-version>           string to set as product version (e.g. "1.0.0")
  <path-to-versioninfo.json>  path to the ``versioninfo.json`` file containing static metadata
  <output-path>               directory where the generated ``.syso`` files should be placed

The created ``.syso`` files are named as ``resource_windows_<arch>.syso``. This
helps Go compiler to pick the correct file based on the target platform and
architecture.
"@

$ErrorActionPreference = "Stop"

$_file_version = $args[0]
if ([string]::IsNullOrEmpty($_file_version)) {
    Write-Host "error: file-version argument is missing"
    Write-Host $_usage
    exit 1
}

$_product_version = $args[1]
if ([string]::IsNullOrEmpty($_product_version)) {
    Write-Host "error: product-version argument is missing"
    Write-Host $_usage
    exit 1
}

$_versioninfo_path = $args[2]
if ([string]::IsNullOrEmpty($_versioninfo_path)) {
    Write-Host "error: path to versioninfo.json is missing"
    Write-Host $_usage
    exit 1
}

if (-not (Test-Path $_versioninfo_path)) {
    Write-Host "error: path to versioninfo.json '$_versioninfo_path' is not a file"
    Write-Host $_usage
    exit 1
}

$_output = $args[3]
if ([string]::IsNullOrEmpty($_output)) {
    Write-Host "error: output path is missing"
    Write-Host $_usage
    exit 1
}

if (-not (Test-Path $_output -PathType Container)) {
    Write-Host "error: output path '$_output' is not a directory"
    Write-Host $_usage
    exit 1
}

go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.5.0

goversioninfo `
    -64 -arm -platform-specific `
    -file-version "$_file_version" `
    -product-version "$_product_version" `
    "$_versioninfo_path"

Move-Item -Path "resource_windows_*.syso" -Destination "$_output" -Force
