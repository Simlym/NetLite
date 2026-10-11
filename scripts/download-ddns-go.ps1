param(
    [ValidateSet('windows','linux')][string]$OS='windows',
    [ValidateSet('amd64','arm64')][string]$Arch='amd64'
)
$ErrorActionPreference='Stop'
# Pinned release: review config and status compatibility before changing this version.
$version='6.17.7'
$root=Split-Path $PSScriptRoot -Parent
$cpu=if($Arch -eq 'amd64'){'x86_64'}else{'arm64'}
$extension=if($OS -eq 'windows'){'zip'}else{'tar.gz'}
$archiveName="ddns-go_${version}_${OS}_${cpu}.${extension}"
$base="https://github.com/jeessy2/ddns-go/releases/download/v$version"
$stage=Join-Path $root ('tools/ddns-go-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $stage -Force | Out-Null
$archive=Join-Path $stage $archiveName
Invoke-WebRequest "$base/$archiveName" -OutFile $archive -TimeoutSec 60
$checksums=(Invoke-WebRequest "$base/checksums.txt" -TimeoutSec 60).Content
if($checksums -is [byte[]]){$checksums=[Text.Encoding]::UTF8.GetString($checksums)}
$expected=$null
foreach($line in ($checksums -split "`n")){
    if($line.Trim() -match '^([a-fA-F0-9]{64})\s+\*?(.+)$' -and $Matches[2] -eq $archiveName){$expected=$Matches[1];break}
}
if(!$expected){throw 'Release checksum missing for the selected archive'}
$actual=(Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
if($actual -ine $expected){throw 'ddns-go archive checksum mismatch; archive not extracted'}
if($OS -eq 'windows'){
    Expand-Archive -LiteralPath $archive -DestinationPath $stage
    $binaryName='ddns-go.exe'
}else{
    & tar -xzf $archive -C $stage
    if($LASTEXITCODE){throw 'ddns-go extraction failed'}
    $binaryName='ddns-go'
}
$binary=Join-Path $stage $binaryName
if(!(Test-Path -LiteralPath $binary)){throw 'Expected ddns-go executable missing'}
$destination=Join-Path $root "tools/ddns-go-v$version-$OS-$Arch"
New-Item -ItemType Directory -Path $destination -Force | Out-Null
Copy-Item -LiteralPath $binary -Destination (Join-Path $destination $binaryName) -Force
# Keep the upstream copyright notice with each downloaded engine.
$license=Join-Path $stage 'LICENSE'
if(!(Test-Path -LiteralPath $license)){throw 'Upstream license missing'}
Copy-Item -LiteralPath $license -Destination (Join-Path $destination 'LICENSE') -Force
Write-Host "Verified ddns-go v$version ($OS/$Arch): $destination"
