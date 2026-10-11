param([ValidateSet('windows','linux')][string]$OS='linux',[ValidateSet('amd64','arm64')][string]$Arch='amd64')
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
New-Item -ItemType Directory -Path "$root/dist","$root/tools" -Force | Out-Null
$oldBin=$env:GOBIN;$oldOS=$env:GOOS;$oldArch=$env:GOARCH;$oldCGO=$env:CGO_ENABLED
try {
 $env:GOBIN="$root/tools";Remove-Item Env:GOOS,Env:GOARCH -ErrorAction SilentlyContinue
 go install github.com/caddyserver/xcaddy/cmd/xcaddy@v0.4.5
 if ($LASTEXITCODE) {throw 'xcaddy installation failed'}
 $env:GOOS=$OS;$env:GOARCH=$Arch;$env:CGO_ENABLED='0'
 $output="$root/dist/caddy-$OS-$Arch";if($OS -eq 'windows') {$output+='.exe'}
 & "$root/tools/xcaddy.exe" build v2.11.7 --with github.com/caddy-dns/cloudflare@v0.2.4 --output $output
 if ($LASTEXITCODE) {throw 'Caddy build failed'}
 if ($OS -eq 'windows') {Copy-Item -LiteralPath $output -Destination "$root/tools/caddy.exe"}
} finally {$env:GOBIN=$oldBin;$env:GOOS=$oldOS;$env:GOARCH=$oldArch;$env:CGO_ENABLED=$oldCGO}
