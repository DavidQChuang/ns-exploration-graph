param([ValidateSet('start','stop','status')][string]$Action='start',[int]$Port=55439)
$ErrorActionPreference='Stop'
$repo=Split-Path $PSScriptRoot -Parent
$pgbin=Join-Path $repo '.local\pgsql\bin'
$data=Join-Path $repo '.local\pgdata'
$log=Join-Path $repo '.local\postgres.log'
if (!(Test-Path (Join-Path $pgbin 'pg_ctl.exe'))) { throw 'Portable PostgreSQL missing. Follow scripts/README.md.' }
if ($Action -eq 'stop') { & (Join-Path $pgbin 'pg_ctl.exe') -D $data -m fast -w stop; if ($LASTEXITCODE) { throw 'PostgreSQL stop failed' }; exit }
if ($Action -eq 'status') { & (Join-Path $pgbin 'pg_ctl.exe') -D $data status; exit $LASTEXITCODE }
if (!(Test-Path (Join-Path $data 'PG_VERSION'))) { & (Join-Path $pgbin 'initdb.exe') -D $data -U postgres -A trust --encoding=UTF8 --locale=C; if ($LASTEXITCODE) { throw 'initdb failed' } }
& (Join-Path $pgbin 'pg_ctl.exe') -D $data status *> $null
if ($LASTEXITCODE -ne 0) {
 # PostgreSQL is a child of a hidden helper; no Windows service or global PATH change.
 $proc=Start-Process -FilePath (Join-Path $pgbin 'pg_ctl.exe') -ArgumentList @('-D',('"'+$data+'"'),'-l',('"'+$log+'"'),'-o',('"-h 127.0.0.1 -p '+$Port+'"'),'-w','start') -WindowStyle Hidden -PassThru
 $proc.WaitForExit(); $proc.Refresh()
 if ($proc.ExitCode -ne 0) { throw ('PostgreSQL startup failed. Read '+$log) }
}
$exists=& (Join-Path $pgbin 'psql.exe') -h 127.0.0.1 -p $Port -U postgres -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname='exploration_graph_test'"
if ($LASTEXITCODE) { throw 'Database health check failed' }
if ($exists -ne '1') { & (Join-Path $pgbin 'createdb.exe') -h 127.0.0.1 -p $Port -U postgres exploration_graph_test; if ($LASTEXITCODE) { throw 'createdb failed' } }
$env:GRAPH_TEST_DATABASE_URL="postgres://postgres@127.0.0.1:$Port/exploration_graph_test?sslmode=disable"
Write-Output $env:GRAPH_TEST_DATABASE_URL

