# Run on Windows 10/11 x64 with Go 1.25+ installed.
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

go mod tidy
if ($LASTEXITCODE -ne 0) { throw 'go mod tidy failed' }
go test -count=1 ./...
if ($LASTEXITCODE -ne 0) { throw 'tests failed' }
go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest app.manifest -o cmd/serviceagreement/rsrc_windows_amd64.syso
if ($LASTEXITCODE -ne 0) { throw 'manifest compilation failed' }
New-Item -ItemType Directory -Path stage -Force | Out-Null
go build -trimpath -ldflags '-s -w -H=windowsgui' -o stage/ServiceAgreement.exe ./cmd/serviceagreement
if ($LASTEXITCODE -ne 0) { throw 'Windows GUI build failed' }
Copy-Item config.example.json stage/config.example.json
Copy-Item config.example.json stage/config.json
Copy-Item README.md stage/README.md
Compress-Archive -Path stage/* -DestinationPath ServiceAgreement-windows-amd64.zip -Force
Write-Host 'Created ServiceAgreement-windows-amd64.zip'