param([string]$CaddyBinary='', [string]$Fnpack='', [ValidateSet('amd64','arm64')][string]$Arch='amd64', [switch]$SkipWebBuild)
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
Set-Location $root
if (!$Fnpack) {$Fnpack=Join-Path $root 'tools/fnpack.exe'}
if (!$CaddyBinary) {$CaddyBinary=Join-Path $root ('dist/caddy-linux-'+$Arch)}
if (!(Test-Path $CaddyBinary)) {throw 'Supply a standard Linux Caddy binary; see scripts/build-caddy.ps1.'}
$CaddyBinary=(Resolve-Path -LiteralPath $CaddyBinary).Path
$Fnpack=(Resolve-Path -LiteralPath $Fnpack).Path
$header=[IO.File]::ReadAllBytes($CaddyBinary)
if ($header.Length -lt 20 -or $header[0] -ne 127 -or $header[1] -ne 69 -or $header[2] -ne 76 -or $header[3] -ne 70) {throw 'Caddy must be Linux ELF, not Windows executable'}
$machine=[BitConverter]::ToUInt16($header,18)
if (($Arch -eq 'amd64' -and $machine -ne 62) -or ($Arch -eq 'arm64' -and $machine -ne 183)) {throw 'Caddy ELF architecture mismatch'}
New-Item -ItemType Directory -Path dist -Force | Out-Null
if ($SkipWebBuild) { if (!(Test-Path 'web/dist/index.html')) {throw 'Build the frontend before using -SkipWebBuild'} } else {
Push-Location web
try {npm.cmd ci; if ($LASTEXITCODE) {throw 'npm ci failed'}; npm.cmd run build; if ($LASTEXITCODE) {throw 'UI build failed'}} finally {Pop-Location}
}
$oldOS=$env:GOOS;$oldArch=$env:GOARCH;$oldCGO=$env:CGO_ENABLED
try {$env:GOOS='linux';$env:GOARCH=$Arch;$env:CGO_ENABLED='0';go build -trimpath -ldflags '-s -w' -o "dist/netlite-linux-$Arch" ./cmd/netlite;if($LASTEXITCODE){throw 'Backend build failed'}}finally{$env:GOOS=$oldOS;$env:GOARCH=$oldArch;$env:CGO_ENABLED=$oldCGO}
# Stage a fresh copy; the tracked packaging template never contains binaries.
$stage=Join-Path $root ('dist/fpk-'+$Arch+'-'+[guid]::NewGuid().ToString('N'))
Copy-Item -LiteralPath (Join-Path $root 'packaging/fnos/netlite') -Destination $stage -Recurse
New-Item -ItemType Directory -Path "$stage/app/bin","$stage/app/web" -Force | Out-Null
Copy-Item -LiteralPath "dist/netlite-linux-$Arch" -Destination "$stage/app/bin/netlite"
Copy-Item -LiteralPath $CaddyBinary -Destination "$stage/app/bin/caddy"
New-Item -ItemType Directory -Path "$stage/app/licenses" -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $root 'THIRD_PARTY_NOTICES.md') -Destination "$stage/app/licenses/THIRD_PARTY_NOTICES.md"
Copy-Item -Path 'web/dist/*'  -Destination "$stage/app/web" -Recurse
if ($Arch -eq 'arm64') {(Get-Content -LiteralPath "$stage/manifest" -Raw).Replace('platform=x86','platform=arm') | Set-Content -LiteralPath "$stage/manifest" -Encoding utf8NoBOM}
Push-Location $stage
try {& $Fnpack build; if ($LASTEXITCODE) {throw 'fnpack build failed'}; & python (Join-Path $root 'scripts/normalize-fpk.py') 'netlite.fpk'; if ($LASTEXITCODE) {throw 'FPK permission/checksum verification failed'};Copy-Item -LiteralPath 'netlite.fpk' -Destination (Join-Path $root "dist/netlite-0.1.4-$Arch.fpk")}finally{Pop-Location}
Write-Host "FPK: $root/dist/netlite-0.1.4-$Arch.fpk"

