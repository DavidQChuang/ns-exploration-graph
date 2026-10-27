param([ValidateSet('all','go','postgres')][string]$Tool='all')
$ErrorActionPreference='Stop'
$repo=Split-Path $PSScriptRoot -Parent
$local=Join-Path $repo '.local'
New-Item -ItemType Directory -Force $local | Out-Null
$downloads=@(
 @{Tool='go';Name='go1.27.3.windows-amd64.zip';URL='https://go.dev/dl/go1.27.3.windows-amd64.zip';SHA256='f26081f438319757f8a1b46d7d0b2c5ec7583d4038e485e0aa725cfde7885fff'},
 @{Tool='postgres';Name='postgresql-17.11-5-windows-x64-binaries.zip';URL='https://get.enterprisedb.com/postgresql/postgresql-17.11-5-windows-x64-binaries.zip';SHA256='80379b2c04d51c30225532e0ae04509899141e9957ed096fe749d7fd9df8f82f'}
)
foreach($download in $downloads) {
 if($Tool -ne 'all' -and $Tool -ne $download.Tool){continue}
 $archive=Join-Path $local $download.Name
 if(!(Test-Path $archive)){Invoke-WebRequest $download.URL -OutFile $archive}
 $actual=(Get-FileHash $archive -Algorithm SHA256).Hash.ToLower()
 if($actual -ne $download.SHA256){throw ('Checksum mismatch: '+$archive)}
 if($download.Tool -eq 'postgres' -and (Test-Path (Join-Path $local 'pgdata\postmaster.pid'))){throw 'Stop and inspect the project database before replacing PostgreSQL binaries.'}
 Expand-Archive $archive -DestinationPath $local -Force
 Write-Output ($download.Tool+' verified '+$actual)
}
