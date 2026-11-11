# kube-onramp bootstrap for Windows (PowerShell 5.1 or newer).
# It downloads kube-onramp, checks its SHA-256 against checksums.txt, puts it
# into %LOCALAPPDATA%\kube-onramp\bin and starts it.
#
# Use (one line):
#   [Net.ServicePointManager]::SecurityProtocol = 'Tls12'; & ([scriptblock]::Create((irm https://github.com/Sten-Qy-Li/kube-onramp/releases/latest/download/install.ps1))) -Team <team>
#
# Rules for this file (docs/SPEC.md, section 5):
# - pure ASCII: PowerShell 5.1 may decode a downloaded script with a legacy code page;
# - never call 'exit': the script runs inside the user's own PowerShell session;
# - no admin rights, no execution-policy changes.
param(
    [string]$Team,
    [string]$StagingNamespace,
    [string]$ProdNamespace,
    [switch]$Minimal,
    [switch]$Yes,
    [switch]$DryRun,
    [switch]$Relogin,
    [switch]$NoColor,
    [switch]$Check,
    [switch]$Reset,
    [string]$Version
)

$ErrorActionPreference = 'Stop'
# The progress bar makes Invoke-WebRequest very slow on PowerShell 5.1.
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$Repo = 'Sten-Qy-Li/kube-onramp'
# The release workflow writes the release tag here (docs/DECISIONS.md, D14).
$Tag = '__KUBE_ONRAMP_TAG__'
$Asset = 'kube-onramp_windows_amd64.exe'

$KorHome = $env:KUBE_ONRAMP_HOME
if (-not $KorHome) { $KorHome = Join-Path $env:LOCALAPPDATA 'kube-onramp' }
$BinDir = Join-Path $KorHome 'bin'
$TmpDir = Join-Path $KorHome 'tmp'
$LogDir = Join-Path $KorHome 'logs'

function Get-OsName {
    try {
        $cv = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
        $build = [int]$cv.CurrentBuild
        # ProductName can say "Windows 10" on Windows 11; the build number decides.
        $family = 'Windows 10'
        if ($build -ge 22000) { $family = 'Windows 11' }
        $edition = ($cv.ProductName -replace '^Windows (10|11) ?', '')
        return ("$family $edition $($cv.DisplayVersion) x64 (build $build)" -replace '\s+', ' ')
    } catch {
        return 'Windows (unknown version)'
    }
}

function Hide-Personal([string]$Text) {
    if ($env:USERPROFILE) { $Text = $Text.Replace($env:USERPROFILE, '~') }
    if ($env:USERNAME -and $env:USERNAME.Length -ge 3) { $Text = $Text -replace ('\b' + [regex]::Escape($env:USERNAME) + '\b'), '<user>' }
    return $Text
}

# Show the standard error report, copy it to the clipboard and save it.
function Write-BootError([string]$Code, [string]$Message, [string]$Next, [string]$Detail) {
    $ps = "$($PSVersionTable.PSVersion.Major).$($PSVersionTable.PSVersion.Minor)"
    $text = @(
        '=== kube-onramp error report (paste this into an AI chatbot) ===',
        'kube-onramp installs kubectl and kubelogin (int128) and sets up OIDC access to a Kubernetes cluster.',
        'Trying to: download and start kube-onramp (install.ps1)',
        "Error code: $Code",
        "Message: $Message",
        "Tool: install.ps1 $Tag | OS: $(Get-OsName) | PowerShell $ps | kubectl unknown | kubelogin unknown",
        '--- details (personal data removed) ---',
        $Detail,
        '=== end of report ==='
    ) -join "`r`n"
    $text = Hide-Personal $text

    Write-Host ''
    Write-Host "  $Code`: $Message" -ForegroundColor Red
    Write-Host "  $Next"
    $copied = $false
    try { Set-Clipboard -Value $text; $copied = $true } catch { }
    $saved = $null
    try {
        New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
        $saved = Join-Path $LogDir ('report-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.txt')
        [IO.File]::WriteAllText($saved, $text)
    } catch { $saved = $null }
    Write-Host ''
    if ($copied) { Write-Host '  The report is copied. Paste it into Slack or into an AI chatbot and ask for help.' }
    if ($saved) { Write-Host "  The report is also saved here: $saved" }
    if (-not $copied -and -not $saved) { Write-Host $text }
}

# 1. Check the system.
$arch = $env:PROCESSOR_ARCHITEW6432
if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }
if (-not [Environment]::Is64BitOperatingSystem -or $arch -ne 'AMD64') {
    Write-BootError 'E-BOOT-OS' "kube-onramp needs 64-bit Windows on an Intel or AMD processor. This computer is: $arch." 'Use another computer, or ask a TA for help.' "PROCESSOR_ARCHITECTURE=$arch"
    return
}

# 2. Choose the release.
if ($Version) {
    $base = "https://github.com/$Repo/releases/download/$Version"
} elseif ($Tag.StartsWith('__')) {
    $base = "https://github.com/$Repo/releases/latest/download"
} else {
    $base = "https://github.com/$Repo/releases/download/$Tag"
}

# 3. Download kube-onramp and checksums.txt.
Write-Host 'Downloading kube-onramp...'
try {
    New-Item -ItemType Directory -Force -Path $BinDir, $TmpDir | Out-Null
    $sums = Join-Path $TmpDir 'checksums.txt'
    $part = Join-Path $TmpDir "$Asset.part"
    Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile $sums
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$Asset" -OutFile $part
} catch {
    Write-BootError 'E-BOOT-DL' 'kube-onramp could not be downloaded.' 'Check that you are online, then paste the install line again.' "$base/$Asset`r`n$($_.Exception.Message)"
    return
}

# 4. Check the SHA-256.
$want = $null
foreach ($line in Get-Content -LiteralPath $sums) {
    if ($line -match ('^([0-9a-fA-F]{64})\s+\*?' + [regex]::Escape($Asset) + '$')) { $want = $Matches[1].ToLower() }
}
$got = (Get-FileHash -Algorithm SHA256 -LiteralPath $part).Hash.ToLower()
if (-not $want -or $want -ne $got) {
    Remove-Item -Force -LiteralPath $part -ErrorAction SilentlyContinue
    Write-BootError 'E-BOOT-SHA' 'The downloaded file is not the expected file (the checksum is wrong). It was deleted.' 'Paste the install line again. If it happens again, send the report to a TA.' "want=$want`r`ngot=$got`r`n$base/$Asset"
    return
}

# 5. Put it into place in one step. A running old copy can be renamed, not replaced.
$exe = Join-Path $BinDir 'kube-onramp.exe'
try {
    try {
        Move-Item -Force -LiteralPath $part -Destination $exe
    } catch {
        $old = "$exe.old"
        Remove-Item -Force -LiteralPath $old -ErrorAction SilentlyContinue
        Move-Item -Force -LiteralPath $exe -Destination $old
        Move-Item -Force -LiteralPath $part -Destination $exe
    }
} catch {
    Write-BootError 'E-BOOT-INSTALL' 'kube-onramp could not be saved in its folder.' 'Close other windows that run kube-onramp, then paste the install line again.' $_.Exception.Message
    return
}

# 6. Start it with the same options.
$kargs = @()
if ($Check) { $kargs += 'check' } elseif ($Reset) { $kargs += 'reset' }
if ($Team) { $kargs += @('--team', $Team) }
if ($StagingNamespace) { $kargs += @('--staging-namespace', $StagingNamespace) }
if ($ProdNamespace) { $kargs += @('--prod-namespace', $ProdNamespace) }
if ($Minimal) { $kargs += '--minimal' }
if ($Yes) { $kargs += '--yes' }
if ($DryRun) { $kargs += '--dry-run' }
if ($Relogin) { $kargs += '--relogin' }
if ($NoColor) { $kargs += '--no-color' }
try {
    & $exe @kargs
} catch {
    Write-BootError 'E-BOOT-RUN' 'kube-onramp was downloaded but could not start. An antivirus program may have blocked it.' 'Send the report to a TA.' $_.Exception.Message
    return
}
