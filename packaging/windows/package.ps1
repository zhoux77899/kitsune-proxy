[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$OutputDirectory
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$outputPath = if ([IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory
} else {
    Join-Path $repoRoot $OutputDirectory
}
$stagingPath = Join-Path ([IO.Path]::GetTempPath()) ("kitsune-proxy-windows-" + [Guid]::NewGuid())

New-Item -ItemType Directory -Force $outputPath | Out-Null
New-Item -ItemType Directory -Force $stagingPath | Out-Null

Push-Location $repoRoot
try {
    $releaseExecutable = Join-Path $outputPath "kitsune-proxy-windows-amd64.exe"
    go build `
        -trimpath `
        -ldflags "-s -w -H=windowsgui" `
        -o $releaseExecutable `
        ./cmd/kitsune-proxy

    Copy-Item $releaseExecutable (Join-Path $stagingPath "kitsune-proxy.exe")
    Copy-Item README.md, LICENSE -Destination $stagingPath
    Compress-Archive `
        -Path (Join-Path $stagingPath "*") `
        -DestinationPath (Join-Path $outputPath "kitsune-proxy-windows-amd64.zip") `
        -Force
} finally {
    Pop-Location
    Remove-Item -LiteralPath $stagingPath -Recurse -Force -ErrorAction SilentlyContinue
}
