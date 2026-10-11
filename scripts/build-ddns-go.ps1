param([string]$Source='E:\git\ddns-go',[ValidateSet('windows','linux')][string]$OS='windows',[ValidateSet('amd64','arm64')][string]$Arch='amd64')
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
$sourcePath=(Resolve-Path -LiteralPath $Source).Path
$tag=& git -C $sourcePath describe --tags --exact-match
if($LASTEXITCODE -or $tag -ne 'v6.17.7'){throw 'Expected ddns-go v6.17.7; review adapter compatibility before changing upstream version'}
$changes=& git -C $sourcePath status --porcelain
if($LASTEXITCODE -or $changes){throw 'Upstream checkout must be clean; existing changes are preserved'}
$extension=if($OS -eq 'windows'){'.exe'}else{''}
$destination=Join-Path $root "tools/ddns-go-$OS-$Arch$extension"
New-Item -ItemType Directory -Force -Path (Split-Path $destination) | Out-Null
$overlayStage=Join-Path $root ('tools/ddns-go-overlay-'+[guid]::NewGuid().ToString('N'))
& python (Join-Path $root 'scripts/prepare-ddns-go-overlay.py') $sourcePath $overlayStage
if($LASTEXITCODE){throw 'ddns-go overlay generation failed'}
$overlay=Join-Path $overlayStage 'overlay.json'
$savedOS=$env:GOOS;$savedArch=$env:GOARCH;$savedCGO=$env:CGO_ENABLED
Push-Location $sourcePath
try {
    & go test -overlay $overlay ./config -run '^TestNetLiteBridge$' -count=1
    if($LASTEXITCODE){throw 'ddns-go bridge verification failed'}
    $env:GOOS=$OS;$env:GOARCH=$Arch;$env:CGO_ENABLED='0'
    & go build -overlay $overlay -trimpath -ldflags '-s -w -X main.version=v6.17.7-netlite.1' -o $destination .
    if($LASTEXITCODE){throw 'ddns-go build failed'}
    Copy-Item -LiteralPath (Join-Path $sourcePath 'LICENSE') -Destination (Join-Path $root 'tools/ddns-go-LICENSE') -Force
}finally{Pop-Location;$env:GOOS=$savedOS;$env:GOARCH=$savedArch;$env:CGO_ENABLED=$savedCGO}
Write-Host "Built $tag ($OS/$Arch): $destination"
