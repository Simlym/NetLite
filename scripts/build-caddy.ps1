param([ValidateSet('windows','linux')][string]$OS='linux',[ValidateSet('amd64','arm64')][string]$Arch='amd64')
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
New-Item -ItemType Directory -Path "$root/dist","$root/tools" -Force | Out-Null
$stage=Join-Path $root ('tools/caddy-build-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $stage -Force | Out-Null
$oldOS=$env:GOOS;$oldArch=$env:GOARCH;$oldCGO=$env:CGO_ENABLED
try {
 Remove-Item Env:GOOS,Env:GOARCH -ErrorAction SilentlyContinue
 Push-Location $stage
 try {
  go mod init netlite-caddy-build
  if($LASTEXITCODE){throw 'Caddy module initialization failed'}
  go get github.com/caddyserver/caddy/v2/cmd/caddy@v2.11.7
  if($LASTEXITCODE){throw 'Caddy dependency preparation failed'}
  $env:GOOS=$OS;$env:GOARCH=$Arch;$env:CGO_ENABLED='0'
  $output="$root/dist/caddy-$OS-$Arch";if($OS -eq 'windows'){$output+='.exe'}
  go build -trimpath -ldflags '-s -w' -o $output github.com/caddyserver/caddy/v2/cmd/caddy
  if($LASTEXITCODE){throw 'Standard Caddy build failed'}
  if($OS -eq 'windows'){try{Copy-Item -LiteralPath $output -Destination "$root/tools/caddy.exe"}catch{Write-Warning "New binary: $output; restart NetLite to use it."}}
 }finally{Pop-Location}
}finally{$env:GOOS=$oldOS;$env:GOARCH=$oldArch;$env:CGO_ENABLED=$oldCGO;Remove-Item -LiteralPath $stage -Recurse -Force}
