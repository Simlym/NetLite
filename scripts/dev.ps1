param([switch]$NoCaddy)
$ErrorActionPreference='Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
if (!(Test-Path 'web/dist/index.html')) { Push-Location web; npm.cmd ci; npm.cmd run build; if ($LASTEXITCODE) {throw 'Frontend build failed'}; Pop-Location }
$caddyPath=if(Test-Path 'dist/caddy-windows-amd64.exe'){'dist/caddy-windows-amd64.exe'}else{'tools/caddy.exe'}
$argsList=@('run','./cmd/netlite','--web','web/dist','--caddy',(Join-Path (Get-Location) $caddyPath))
if ($NoCaddy) {$argsList+='--no-caddy'}
& go @argsList

