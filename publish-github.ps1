# Optional fallback if ChatGPT GitHub connector cannot write to this repository.
# Use only for the initially empty Postnn/ServiceAgreement repository.
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
if (!(Get-Command git -ErrorAction SilentlyContinue)) { throw 'Git is required' }
if (!(Test-Path '.git')) { git init -b main }
git remote get-url origin *> $null
if ($LASTEXITCODE -ne 0) { git remote add origin 'https://github.com/Postnn/ServiceAgreement.git' }
git add .
git commit -m 'feat: initial standalone Windows SQL service agreements editor'
if ($LASTEXITCODE -ne 0) { throw 'git commit failed (configure Git identity first)' }
git push -u origin main
if ($LASTEXITCODE -ne 0) { throw 'git push failed; check GitHub authentication and repository access' }
Write-Host 'Sources published to GitHub. Check Actions and Releases for the Windows ZIP.'