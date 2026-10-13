# Isolated real-engine check: the IP source cannot return an address, so no DNS
# provider query/update should be sent. Do not use real credentials in this test.
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
$data=Join-Path $root ('data/ddns-status-'+[guid]::NewGuid().ToString('N'))
$base='http://127.0.0.1:19780'
$process=$null
foreach($port in @(19780,21790,21999)) {
 if(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue){throw "Test port $port is already occupied"}
}
New-Item -ItemType Directory -Path $data -Force | Out-Null
Push-Location $root
try {
 go build -o bin/netlite.exe ./cmd/netlite
 if($LASTEXITCODE){throw 'Backend build failed'}
 $process=Start-Process -FilePath (Join-Path $root 'bin/netlite.exe') -ArgumentList @('--auth-mode','token','--listen','127.0.0.1:19780','--data',('"'+$data+'"'),'--web',('"'+$root+'/web/dist"'),'--caddy',('"'+$root+'/tools/caddy.exe"'),'--caddy-admin','127.0.0.1:21790') -WindowStyle Hidden -PassThru -RedirectStandardOutput "$data/stdout.log" -RedirectStandardError "$data/stderr.log"
 $ready=$false
 for($i=0;$i -lt 30;$i++){Start-Sleep -Milliseconds 200;try {Invoke-RestMethod "$base/healthz" | Out-Null;$ready=$true;break}catch{}}
 if(!$ready){throw 'Test backend did not become ready'}
 $token=Get-Content -LiteralPath "$data/admin-token" -Raw
 $headers=@{Authorization="Bearer $token"}
 function Put-TestObject($path,$body){Invoke-RestMethod "$base/api/dns/$path" -Headers $headers -Method Put -ContentType 'application/json' -Body ($body|ConvertTo-Json -Depth 10) | Out-Null}
 Put-TestObject 'accounts/statusdemo' @{id='statusdemo';name='isolated demo';provider='dnspod';credential_id='not-a-real-id';secret='not-a-real-token'}
 Put-TestObject 'tasks/statusdemo' @{id='statusdemo';name='IP failure test';account_id='statusdemo';ipv4_domains=@('status.example.invalid');ipv6_domains=@();ipv4_url='http://127.0.0.1:21999/ip';ipv6_url='';interval_seconds=300;ttl='600';enabled=$true}
 $state=$null
 for($i=0;$i -lt 90;$i++){Start-Sleep -Milliseconds 500;$snapshot=Invoke-RestMethod "$base/api/dns/" -Headers $headers;$state=$snapshot.tasks[0].state;if($state.result_known){break}}
 if(!$state.result_known -or $state.ipv4_result -ne 'ip_failed' -or $state.ipv4 -or $state.provider_requests -ne 0 -or $state.update_requests -ne 0){throw 'Real engine failed to report IP failure without DNS requests'}
 Invoke-RestMethod "$base/api/dns/tasks/statusdemo" -Headers $headers -Method Delete | Out-Null
 Write-Host 'PASS: native DDNS worker -> authenticated API; IP failure, check time and zero provider/update requests.'
}finally{
 if($headers){try{Invoke-RestMethod "$base/api/dns/tasks/statusdemo" -Headers $headers -Method Delete | Out-Null}catch{}}
 if($process -and !$process.HasExited){try{Invoke-WebRequest -Method Post 'http://127.0.0.1:21790/stop' | Out-Null}catch{};$process.WaitForExit(3000)|Out-Null;if(!$process.HasExited){Stop-Process -Id $process.Id}}
 Pop-Location
}

