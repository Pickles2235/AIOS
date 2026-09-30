param(
    [Parameter(Mandatory = $true)][string]$Version,
    [Parameter(Mandatory = $true)][string]$OutputDir
)

$ErrorActionPreference = "Stop"
if ($Version -notmatch '^1(?:\.|$)[A-Za-z0-9._-]*$') {
    throw "V1 release version must be 1 or start with 1."
}

$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$Output = [System.IO.Path]::GetFullPath($OutputDir)
$Arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "X64") { "amd64" } else { throw "unsupported Windows release architecture" }
$Package = "aios-$Version-windows-$Arch"
$StageRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("aios-release-" + [guid]::NewGuid())
$Stage = Join-Path $StageRoot $Package

try {
    New-Item -ItemType Directory -Force -Path (Join-Path $Stage "bin"), (Join-Path $Stage "docs"), $Output | Out-Null
    Push-Location $Root
    npm --prefix web run build
    if ($LASTEXITCODE -ne 0) { throw "web build failed" }
    $env:CGO_ENABLED = "1"
    go build -trimpath -ldflags "-s -w" -o (Join-Path $Stage "bin/aios.exe") ./cmd/aios
    if ($LASTEXITCODE -ne 0) { throw "Go build failed" }
    Copy-Item README.md, config.example.json -Destination $Stage
    Copy-Item docs/* -Destination (Join-Path $Stage "docs") -Recurse
    Copy-Item acceptance -Destination $Stage -Recurse
    Copy-Item cmd/aios/notices/Apache-2.0.txt -Destination (Join-Path $Stage "NOTICE-Apache-2.0.txt")
    $Archive = Join-Path $Output "$Package.zip"
    Compress-Archive -Path $Stage -DestinationPath $Archive -Force
    $Hash = (Get-FileHash -Algorithm SHA256 $Archive).Hash.ToLowerInvariant()
    "$Hash  $([System.IO.Path]::GetFileName($Archive))" | Set-Content -Encoding ascii "$Archive.sha256"
    Write-Output $Archive
} finally {
    Pop-Location -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $StageRoot -ErrorAction SilentlyContinue
}
