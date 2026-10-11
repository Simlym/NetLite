param([switch]$NoCaddy)
$ErrorActionPreference='Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
if (!(Test-Path 'web/dist/index.html')) { Push-Location web; npm.cmd ci; npm.cmd run build; if ($LASTEXITCODE) {throw 'Frontend build failed'}; Pop-Location }
$argsList=@('run','./cmd/netlite','--web','web/dist','--caddy',(Join-Path (Get-Location) 'tools/caddy.exe'))
if ($NoCaddy) {$argsList+='--no-caddy'}
& go @argsList
