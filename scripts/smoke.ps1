$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
Set-Location $root
$data=Join-Path $root ('data/smoke-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path "$root/bin",$data -Force | Out-Null
go build -o bin/netlite.exe ./cmd/netlite
if($LASTEXITCODE){throw 'Build failed'}
$exe=Join-Path $root 'bin/netlite.exe';$caddy=Join-Path $root 'tools/caddy.exe'
$process=$null;$upstream=$null
function Start-NetLite {
 Start-Process -FilePath $exe -ArgumentList @('--listen','127.0.0.1:19780','--data',('"'+$data+'"'),'--web',('"'+$root+'/web/dist"'),'--caddy',('"'+$caddy+'"'),'--caddy-admin','127.0.0.1:21190') -WindowStyle Hidden -PassThru -RedirectStandardOutput "$data/stdout.log" -RedirectStandardError "$data/stderr.log"
}
function Stop-NetLite {
 if($process -and !$process.HasExited){
  # Windows console signals aren't exposed by Start-Process; the test stops Caddy
  # through its dedicated admin API and terminates only its own backend process.
  try {Invoke-WebRequest -Method Post -Uri 'http://127.0.0.1:21190/stop' | Out-Null}catch{}
  $process.WaitForExit(10000) | Out-Null
  if(!$process.HasExited){Stop-Process -Id $process.Id}
 }
}
try {
 $upstream=Start-Process -FilePath 'node' -ArgumentList @((Join-Path $root 'scripts/test-upstream.cjs')) -WindowStyle Hidden -PassThru
 $process=Start-NetLite
 for($i=0;$i -lt 30;$i++){Start-Sleep -Milliseconds 500;if(Test-Path "$data/admin-token"){try{Invoke-WebRequest 'http://127.0.0.1:21190/config/' | Out-Null;break}catch{}}}
 $token=Get-Content -LiteralPath "$data/admin-token" -Raw;$headers=@{Authorization="Bearer $token"}
 try{Invoke-RestMethod 'http://127.0.0.1:19780/api/config' | Out-Null;throw 'Unauthenticated access allowed'}catch{if($_.Exception.Response.StatusCode -ne 401){throw}}
 $cfg=Invoke-RestMethod 'http://127.0.0.1:19780/api/config' -Headers $headers
 $cfg.routes=@(@{id='http';domain='plain.localhost';scheme='http';port=19081;upstream='http://127.0.0.1:19881';certificate_id='';enabled=$true},@{id='https';domain='localhost';scheme='https';port=19444;upstream='http://127.0.0.1:19881';certificate_id='local';enabled=$true})
 $cfg.certificates=@(@{id='local';domains=@('localhost');mode='internal';email='';provider='';token_env='';staging=$false})
 $body=$cfg | ConvertTo-Json -Depth 10
 $saved=Invoke-RestMethod 'http://127.0.0.1:19780/api/config' -Headers $headers -Method Put -ContentType 'application/json' -Body $body
 try{Invoke-RestMethod 'http://127.0.0.1:19780/api/config' -Headers $headers -Method Put -ContentType 'application/json' -Body $body | Out-Null;throw 'Stale update allowed'}catch{if($_.Exception.Response.StatusCode -ne 409){throw}}
 Invoke-RestMethod 'http://127.0.0.1:19780/api/proxy/validate' -Headers $headers -Method Post | Out-Null
 Invoke-RestMethod 'http://127.0.0.1:19780/api/proxy/apply' -Headers $headers -Method Post | Out-Null
 $plain=Invoke-WebRequest 'http://127.0.0.1:19081/hello' -Headers @{Host='plain.localhost'}
 if($plain.Content -notmatch 'upstream:/hello'){throw 'HTTP proxy failed'}
 $secure=$null
 for($i=0;$i -lt 30;$i++){try{$secure=Invoke-WebRequest 'https://localhost:19444/secure' -SkipCertificateCheck;break}catch{Start-Sleep -Milliseconds 500}}
 if(!$secure -or $secure.Content -notmatch 'upstream:/secure'){throw 'HTTPS proxy failed'}
 $saved.routes[0].enabled=$false
 Invoke-RestMethod 'http://127.0.0.1:19780/api/config' -Headers $headers -Method Put -ContentType 'application/json' -Body ($saved | ConvertTo-Json -Depth 10) | Out-Null
 Stop-NetLite
 $process=Start-NetLite
 for($i=0;$i -lt 30;$i++){try{$plain=Invoke-WebRequest 'http://127.0.0.1:19081/restart' -Headers @{Host='plain.localhost'};break}catch{Start-Sleep -Milliseconds 500}}
 if($plain.Content -notmatch 'upstream:/restart'){throw 'Applied configuration not restored'}
 $restored=Invoke-RestMethod 'http://127.0.0.1:19780/api/config' -Headers $headers
 if($restored.routes[0].enabled){throw 'Draft configuration not persisted'}
 Write-Host 'PASS: authentication, revision conflict, HTTP proxy, HTTPS internal CA, restart restores applied config and preserves draft.'
} finally {Stop-NetLite;if($upstream -and !$upstream.HasExited){Stop-Process -Id $upstream.Id}}
