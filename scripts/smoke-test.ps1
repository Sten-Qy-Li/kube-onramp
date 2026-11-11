# End-to-end smoke test on Windows (docs/SPEC.md, section 12):
# setup, setup again (nothing may change), check, reset, then everything
# must be back as it was. Locally it uses a throwaway registry key instead of
# the real user PATH; in CI (CI=true) it tests the real HKCU\Environment.
#
# Usage: ./scripts/smoke-test.ps1 -Binary path\to\kube-onramp.exe
param([Parameter(Mandatory = $true)][string]$Binary)
$ErrorActionPreference = 'Stop'

$work = Join-Path ([IO.Path]::GetTempPath()) ('kor-smoke-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force (Join-Path $work 'kube') | Out-Null
$env:KUBE_ONRAMP_HOME = Join-Path $work 'home'
$env:KUBECONFIG = Join-Path $work 'kube\config'
$fixture = Join-Path $PSScriptRoot '..\internal\kubeconfig\testdata\other.canonical.yaml'
Copy-Item $fixture $env:KUBECONFIG

if ($env:CI -ne 'true') {
    $env:KUBE_ONRAMP_TEST_ENV_KEY = 'kube-onramp-smoke'
    $regPath = "HKCU:\Software\$($env:KUBE_ONRAMP_TEST_ENV_KEY)"
    New-Item -Path $regPath -Force | Out-Null
} else {
    $regPath = 'HKCU:\Environment'
}

function Get-UserPath {
    $k = Get-Item -LiteralPath $regPath
    $v = $k.GetValue('Path', $null, 'DoNotExpandEnvironmentNames')
    if ($null -eq $v) { return '<no value>' }
    return "$($k.GetValueKind('Path'))|$v"
}
function Invoke-Kor([string]$Exe, [string[]]$KArgs) {
    $out = & $Exe @KArgs 2>&1 | Out-String
    $code = $LASTEXITCODE
    Write-Host $out
    return @{ Code = $code; Out = $out }
}
function Assert-True($Cond, [string]$Message) {
    if (-not $Cond) { throw "SMOKE TEST FAILED: $Message" }
}
function Get-Snapshot {
    Get-ChildItem (Join-Path $env:KUBE_ONRAMP_HOME 'bin'), (Join-Path $env:KUBE_ONRAMP_HOME 'state.json'), $env:KUBECONFIG -File |
        ForEach-Object { "$($_.FullName)|$($_.LastWriteTimeUtc.Ticks)|$($_.Length)" }
    Get-UserPath
}

$pathBefore = Get-UserPath
try {
    Write-Host '== 1. setup'
    $r = Invoke-Kor $Binary @('--yes', '--minimal', '--skip-login', '--no-color', '--team', 'ci-test')
    Assert-True ($r.Code -eq 0) "setup exit code $($r.Code)"
    $own = Join-Path $env:KUBE_ONRAMP_HOME 'bin\kube-onramp.exe'
    Assert-True (Test-Path $own) 'kube-onramp was not copied into its bin folder'
    Assert-True ((Get-UserPath) -like "*$($env:KUBE_ONRAMP_HOME)\bin*") 'the bin folder is not on the user PATH'

    Write-Host '== 2. setup again: nothing may change'
    $before = Get-Snapshot
    $r = Invoke-Kor $Binary @('--yes', '--minimal', '--skip-login', '--no-color')
    Assert-True ($r.Code -eq 0) "second setup exit code $($r.Code)"
    Assert-True ($r.Out -match 'Everything is already set up') 'the second run did not say "Everything is already set up"'
    $diff = Compare-Object $before (Get-Snapshot)
    Assert-True (-not $diff) "the second run changed something: $($diff | Out-String)"

    Write-Host '== 3. check'
    $r = Invoke-Kor $own @('check', '--skip-login', '--no-color')
    Assert-True ($r.Code -eq 0) "check exit code $($r.Code)"

    Write-Host '== 4. reset, run by the installed copy'
    $r = Invoke-Kor $own @('reset', '--yes', '--no-color')
    Assert-True ($r.Code -eq 0) "reset exit code $($r.Code)"
    Start-Sleep -Seconds 6 # the helper deletes the running exe after ~3 s

    Write-Host '== 5. everything is back'
    Assert-True ((Get-UserPath) -eq $pathBefore) "the user PATH is not back: '$(Get-UserPath)' instead of '$pathBefore'"
    Assert-True ((Get-FileHash $env:KUBECONFIG).Hash -eq (Get-FileHash $fixture).Hash) 'the kubeconfig is not back to the original'
    Assert-True (-not (Test-Path (Join-Path $env:KUBE_ONRAMP_HOME 'bin'))) 'the bin folder still exists'
    Assert-True (-not (Test-Path (Join-Path $env:KUBE_ONRAMP_HOME 'state.json'))) 'state.json still exists'
    Assert-True (Test-Path (Join-Path $env:KUBE_ONRAMP_HOME 'backups')) 'the backups folder is gone'
    Write-Host 'SMOKE TEST PASSED'
} finally {
    if ($env:CI -ne 'true') { Remove-Item -Path $regPath -Recurse -Force -ErrorAction SilentlyContinue }
}
