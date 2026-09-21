# Quiver installer for Windows.
#
#   irm https://raw.githubusercontent.com/rabbytesoftware/quiver.core/develop/install.ps1 | iex
#
# Targets Windows PowerShell 5.1 and PowerShell 7+ from one file. 5.1 is what
# every Windows box already has, 7 is what people install, and the differences
# that matter here are handled explicitly rather than by picking a side:
#
#   * $IsWindows does not exist before PowerShell 6, so `-not $IsWindows` is
#     true on a real Windows 5.1 host. $env:OS is the check that works on both.
#   * 5.1 negotiates TLS per its .NET default, which on an unpatched box is
#     still TLS 1.0. GitHub requires 1.2, so it is set explicitly.
#   * 5.1's Invoke-WebRequest progress bar costs more time than the download
#     itself on a 50 MB file. $ProgressPreference silences it.
#   * No ternary, no ??, no 3-argument Join-Path: all of those are 7-only.
#
# The file is ASCII only on purpose: 5.1 reads a BOM-less .ps1 as ANSI, so a
# non-ASCII character here would decode differently depending on the machine's
# code page.
#
# Configuration is by environment variable, not parameters. `irm | iex` runs
# this text inside the caller's session, where a param() block is a syntax
# error and $args is not populated, so a -InstallDir switch could never be
# passed through the documented one-liner:
#
#   $env:QUIVER_INSTALL_DIR   install somewhere other than the default
#   $env:QUIVER_TAG           install a specific release, e.g. stable-26.5.1
#
# Everything lives in one function that the last line calls, so a truncated
# download defines a function and runs nothing.

function Install-Quiver {
    # Set here rather than at script scope. Under `irm | iex` this text runs in
    # the caller's own session, where a script-scope assignment would outlive
    # the install and change how every later command in their shell reports
    # errors. A function scope reverts on return, and preference variables are
    # inherited by everything this calls.
    $ErrorActionPreference = 'Stop'

    $repo = 'rabbytesoftware/quiver.core'
    $apiBase = "https://api.github.com/repos/$repo/releases"

    # Every URL taken out of the API response must start with this. A release
    # body is free-form text inside the same JSON document, so pinning the
    # prefix is what stops a crafted release note from redirecting the download.
    $downloadPrefix = "https://github.com/$repo/releases/download/"

    if ($env:OS -ne 'Windows_NT') {
        throw "this is the Windows installer. On macOS and Linux run:`n" +
              "    curl -fsSL https://raw.githubusercontent.com/$repo/develop/install.sh | bash"
    }

    $ProgressPreference = 'SilentlyContinue'
    try {
        [Net.ServicePointManager]::SecurityProtocol =
            [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    } catch {
        # PowerShell 7 on .NET 5+ has no ServicePointManager knob to turn and
        # already negotiates TLS 1.2 or better. Nothing to do.
    }

    $asset = Resolve-QuiverAsset
    Write-Host "Installing quiver for windows/amd64" -ForegroundColor White

    $installDir = Resolve-QuiverInstallDir
    $target = [IO.Path]::Combine($installDir, 'quiver.exe')

    $staging = Join-Path ([IO.Path]::GetTempPath()) ("quiver-install-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $staging -Force | Out-Null
    try {
        $release = Get-QuiverRelease -ApiBase $apiBase -Prefix $downloadPrefix -AssetName $asset

        Write-Step "  Downloading $asset from $($release.Tag)..."
        $binPath = Join-Path $staging $asset
        $sumPath = Join-Path $staging 'checksums.txt'
        Get-QuiverFile -Uri $release.AssetUrl -OutFile $binPath
        Get-QuiverFile -Uri $release.ChecksumsUrl -OutFile $sumPath

        Write-Step '  Verifying sha256...'
        Assert-QuiverChecksum -BinaryPath $binPath -ChecksumsPath $sumPath -AssetName $asset -Tag $release.Tag

        Stop-QuiverDaemon
        Install-QuiverBinary -StagedPath $binPath -Target $target -InstallDir $installDir

        $onPath = Add-QuiverToPath -InstallDir $installDir
        Write-QuiverSuccess -Target $target -Tag $release.Tag -AlreadyOnPath $onPath
    } finally {
        Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

function Write-Step { param([string] $Message) Write-Host $Message -ForegroundColor DarkGray }
function Write-Note { param([string] $Message) Write-Host "warning: $Message" -ForegroundColor Yellow }

# ---------------------------------------------------------------------------
# Platform
# ---------------------------------------------------------------------------

# Resolve-QuiverAsset names the release asset for this machine. quiver.core
# publishes exactly one Windows build, quiver-windows-amd64.exe, so this is
# really an architecture gate rather than a choice.
function Resolve-QuiverAsset {
    # PROCESSOR_ARCHITECTURE reports the architecture of the *process*, so a
    # 32-bit PowerShell host on a 64-bit Windows reports x86 and puts the real
    # answer in PROCESSOR_ARCHITEW6432. Reading the second one first is what
    # makes this correct from either host.
    $arch = $env:PROCESSOR_ARCHITEW6432
    if ([string]::IsNullOrEmpty($arch)) { $arch = $env:PROCESSOR_ARCHITECTURE }
    if ([string]::IsNullOrEmpty($arch)) {
        throw 'cannot determine this machine''s architecture: %PROCESSOR_ARCHITECTURE% is not set.'
    }

    switch ($arch.ToUpperInvariant()) {
        'AMD64' { return 'quiver-windows-amd64.exe' }
        'ARM64' {
            # There is no windows/arm64 asset. Windows 11 on ARM runs x64
            # binaries under emulation, so the amd64 build is the right answer
            # rather than a failure, but it should not happen silently.
            Write-Note 'no native arm64 build is published; installing the amd64 build, which Windows runs under x64 emulation.'
            return 'quiver-windows-amd64.exe'
        }
        default {
            throw "unsupported architecture: $arch. quiver.core publishes a 64-bit Windows build only; there is no 32-bit release."
        }
    }
}

# ---------------------------------------------------------------------------
# Release resolution
# ---------------------------------------------------------------------------

# Get-QuiverRelease reads the real release metadata and pulls the asset URLs out
# of it. URLs are never templated from a version string: a release that renamed
# or dropped an asset has to fail here with a readable message instead of 404ing
# halfway through a download.
function Get-QuiverRelease {
    param(
        [string] $ApiBase,
        [string] $Prefix,
        [string] $AssetName
    )

    if ([string]::IsNullOrEmpty($env:QUIVER_TAG)) {
        # /releases/latest is the current stable release by definition: betas,
        # hotfixes and nightlies are all published as prereleases, which GitHub
        # excludes from this endpoint.
        $url = "$ApiBase/latest"
        Write-Step '  Resolving the latest release...'
    } else {
        $url = "$ApiBase/tags/$($env:QUIVER_TAG)"
        Write-Step "  Resolving release $($env:QUIVER_TAG)..."
    }

    try {
        $release = Invoke-RestMethod -Uri $url -UseBasicParsing -Headers @{ 'Accept' = 'application/vnd.github+json' }
    } catch {
        $status = 0
        try { $status = [int] $_.Exception.Response.StatusCode } catch { }
        if ($status -eq 403 -or $status -eq 429) {
            throw "GitHub's unauthenticated API rate limit is exhausted for this IP address.`n" +
                  "       Wait for it to reset, or download a binary by hand from`n" +
                  "       https://github.com/rabbytesoftware/quiver.core/releases/latest"
        }
        if ($status -eq 404) {
            throw "there is no release tagged $($env:QUIVER_TAG) in this repository"
        }
        throw "could not reach the GitHub release API at ${url}: $($_.Exception.Message)"
    }

    if ([string]::IsNullOrEmpty($release.tag_name)) {
        throw "the release API returned no tag_name; $url may not name a real release"
    }

    # Only assets whose download URL actually belongs to this repository's
    # release downloads are considered, and they are matched on the asset's own
    # file name rather than on a URL built from the tag, so a renamed asset is a
    # clear failure and not a silent 404.
    $assets = @($release.assets | Where-Object { $_.browser_download_url -and $_.browser_download_url.StartsWith($Prefix, [StringComparison]::Ordinal) })

    $binary = $assets | Where-Object { $_.name -eq $AssetName } | Select-Object -First 1
    if (-not $binary) {
        $names = ($assets | ForEach-Object { "         $($_.name)" }) -join "`n"
        throw "release $($release.tag_name) publishes no $AssetName asset.`n       Assets it does publish:`n$names"
    }

    $sums = $assets | Where-Object { $_.name -eq 'checksums.txt' } | Select-Object -First 1
    if (-not $sums) {
        throw "release $($release.tag_name) publishes no checksums.txt, so the download cannot be verified"
    }

    return [PSCustomObject]@{
        Tag          = $release.tag_name
        AssetUrl     = $binary.browser_download_url
        ChecksumsUrl = $sums.browser_download_url
    }
}

function Get-QuiverFile {
    param([string] $Uri, [string] $OutFile)

    try {
        Invoke-WebRequest -Uri $Uri -OutFile $OutFile -UseBasicParsing
    } catch {
        throw "download failed for ${Uri}: $($_.Exception.Message)"
    }
    if (-not (Test-Path -LiteralPath $OutFile)) {
        throw "download produced no file: $Uri"
    }
}

# ---------------------------------------------------------------------------
# Verification
# ---------------------------------------------------------------------------

function Assert-QuiverChecksum {
    param(
        [string] $BinaryPath,
        [string] $ChecksumsPath,
        [string] $AssetName,
        [string] $Tag
    )

    # checksums.txt is produced by `cd bin && sha256sum ./*` in
    # build-assets.yml, so every path in it carries a ./ prefix. Matching on the
    # basename means a change to how that workflow invokes sha256sum cannot
    # quietly skip verification.
    $expected = $null
    foreach ($line in (Get-Content -LiteralPath $ChecksumsPath)) {
        $fields = $line.Trim() -split '\s+'
        if ($fields.Count -lt 2) { continue }
        $name = ($fields[$fields.Count - 1] -split '[\\/]')[-1]
        if ($name -eq $AssetName) {
            $expected = $fields[0]
            break
        }
    }

    if ([string]::IsNullOrEmpty($expected)) {
        throw "checksums.txt in $Tag has no entry for $AssetName; refusing to install an unverified binary"
    }

    $actual = (Get-FileHash -LiteralPath $BinaryPath -Algorithm SHA256).Hash
    if ($actual -ine $expected) {
        throw "checksum mismatch for $AssetName, refusing to install.`n" +
              "       expected $($expected.ToLowerInvariant())`n" +
              "       actual   $($actual.ToLowerInvariant())`n" +
              "       The download was corrupted or tampered with. Nothing has been installed."
    }
}

# ---------------------------------------------------------------------------
# Installation
# ---------------------------------------------------------------------------

# Resolve-QuiverInstallDir picks %LOCALAPPDATA%\Quiver\bin.
#
# quiver.desktop's per-user NSIS installer puts quiverdesktop.exe directly in
# %LOCALAPPDATA%\Quiver (its ARROW.md preinstalled probe tests that exact path;
# the per-machine MSI uses %ProgramFiles%\Quiver). Sharing that Quiver root
# keeps one per-user home for everything Quiver installs, and the bin
# subdirectory earns its place twice over: the desktop probe's exact test for
# %LOCALAPPDATA%\Quiver\quiverdesktop.exe can never be confused by a sibling
# file, and putting bin on PATH exposes quiver.exe without also exposing a GUI
# executable that has no business being invoked from a prompt.
#
# LOCALAPPDATA rather than %USERPROFILE%\Documents\.quiver, which is where
# metadata.yaml puts quiver.core's Windows *data* home: Documents is a
# user-visible, frequently cloud-synced folder, and an executable on PATH does
# not belong in one. This is an install location, not a state directory.
#
# No admin rights are needed anywhere in here, and none are asked for: a
# per-user install needs no privileges, and a UAC prompt out of a script the
# user just piped into their shell is not a trade worth making.
function Resolve-QuiverInstallDir {
    if (-not [string]::IsNullOrEmpty($env:QUIVER_INSTALL_DIR)) {
        return $env:QUIVER_INSTALL_DIR
    }

    $localAppData = $env:LOCALAPPDATA
    if ([string]::IsNullOrEmpty($localAppData)) {
        if ([string]::IsNullOrEmpty($env:USERPROFILE)) {
            throw 'cannot pick an install directory: neither %LOCALAPPDATA% nor %USERPROFILE% is set. Set $env:QUIVER_INSTALL_DIR and re-run.'
        }
        $localAppData = [IO.Path]::Combine($env:USERPROFILE, 'AppData', 'Local')
    }

    # [IO.Path]::Combine rather than Join-Path: Join-Path goes through the
    # PowerShell provider stack and resolves the drive, so it throws on a root
    # whose drive is not currently mounted. This is composing a path that does
    # not exist yet, which is a string operation and nothing more. The 3-argument
    # form is a .NET overload, not Join-Path's 3-argument form, which is 7-only.
    return [IO.Path]::Combine($localAppData, 'Quiver', 'bin')
}

# Stop-QuiverDaemon mirrors what the Makefile's install target does on Unix, for
# the same reason its comment gives: replacing the binary underneath a daemon
# that keeps serving leaves an old daemon talking to a new CLI, and that version
# skew surfaces as decode panics rather than as a clean error. On Windows it is
# also load-bearing, not just hygiene, because a running .exe is held under an
# exclusive lock that blocks overwriting it at all.
#
# The pid file is the one daemon.NewManager writes, which is under
# os.UserHomeDir() and so %USERPROFILE%\.quiver, not the Documents-based data
# home metadata.yaml configures. The process name is checked before anything is
# signalled: Windows recycles pids quickly and a stale pid file must never cost
# an unrelated process its life.
function Stop-QuiverDaemon {
    if ([string]::IsNullOrEmpty($env:USERPROFILE)) { return }
    $pidFile = [IO.Path]::Combine($env:USERPROFILE, '.quiver', 'quiver.pid')
    if (-not (Test-Path -LiteralPath $pidFile)) { return }

    $raw = (Get-Content -LiteralPath $pidFile -TotalCount 1 -ErrorAction SilentlyContinue)
    if ($raw -isnot [string]) { return }
    $raw = $raw.Trim()
    if ($raw -notmatch '^\d+$') { return }

    $daemon = Get-Process -Id ([int] $raw) -ErrorAction SilentlyContinue
    if (-not $daemon) { return }
    if ($daemon.ProcessName -ne 'quiver') { return }

    # Windows has no SIGTERM to send from here, so this is a hard stop. The
    # daemon is restarted by the next CLI command that needs it.
    Write-Step "  Stopping the running daemon (pid $raw)..."
    Stop-Process -Id $daemon.Id -Force -ErrorAction SilentlyContinue
    try { $daemon.WaitForExit(10000) | Out-Null } catch { }
}

function Install-QuiverBinary {
    param([string] $StagedPath, [string] $Target, [string] $InstallDir)

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null

    # Left over from a previous run that could not delete a locked binary. This
    # is the cleanup half of the rename-aside below, and doing it on every run
    # is what keeps repeated installs from accumulating copies.
    Get-ChildItem -LiteralPath $InstallDir -Filter 'quiver.exe.old-*' -ErrorAction SilentlyContinue |
        Remove-Item -Force -ErrorAction SilentlyContinue

    # -Force overwrites in place, so there is never a window where the install
    # directory holds no quiver.exe at all. Deleting first and then moving would
    # leave the user with nothing if the move went on to fail.
    $replaced = $false
    try {
        Move-Item -LiteralPath $StagedPath -Destination $Target -Force -ErrorAction Stop
        $replaced = $true
    } catch {
        # Only an existing binary held open explains this. Anything else (an
        # unwritable directory, a vanished staging file) is a real failure.
        if (-not (Test-Path -LiteralPath $Target)) { throw }
    }
    if ($replaced) { return }

    # A quiver process this script could not identify is running out of the
    # target. Windows refuses to overwrite a running image but does allow
    # renaming one, since that only rewrites a directory entry. Moving it aside
    # frees the name now, and the cleanup above deletes it on the next run.
    $aside = 'quiver.exe.old-' + (Get-Date -Format 'yyyyMMddHHmmss')
    try {
        Rename-Item -LiteralPath $Target -NewName $aside -Force -ErrorAction Stop
    } catch {
        # Renaming failing too means this is not a lock at all. Say both
        # possibilities rather than guessing, and pass the real reason through.
        throw "cannot replace $Target. Either a running quiver process is holding it, " +
              "or $InstallDir is not writable. Close any running quiver and try again, " +
              "or set `$env:QUIVER_INSTALL_DIR to somewhere you can write.`n" +
              "       ($($_.Exception.Message))"
    }
    Move-Item -LiteralPath $StagedPath -Destination $Target -Force
}

# ---------------------------------------------------------------------------
# PATH
# ---------------------------------------------------------------------------

function Test-OnPath {
    param([string] $Directory)

    $wanted = $Directory.TrimEnd('\')
    foreach ($scope in @('User', 'Machine')) {
        $value = [Environment]::GetEnvironmentVariable('Path', $scope)
        if ([string]::IsNullOrEmpty($value)) { continue }
        foreach ($entry in $value.Split(';')) {
            if ([string]::IsNullOrWhiteSpace($entry)) { continue }
            if ([Environment]::ExpandEnvironmentVariables($entry).Trim().TrimEnd('\') -ieq $wanted) {
                return $true
            }
        }
    }
    return $false
}

# Add-QuiverToPath appends the install directory to the *user* PATH, persistently.
#
# Deliberately not [Environment]::SetEnvironmentVariable(...,'User'): that call
# writes the value back as REG_SZ, and a user PATH is normally REG_EXPAND_SZ
# holding entries like %USERPROFILE%\AppData\Local\Microsoft\WindowsApps. Writing
# it back as a plain string stops those expanding and silently breaks whatever
# they pointed at. Going through the registry directly preserves the value kind.
#
# The cost of not using that API is that it is also what broadcasts
# WM_SETTINGCHANGE, so the broadcast is done explicitly below. Without it, a new
# terminal started from Explorer inherits Explorer's cached environment and does
# not see the new PATH until the user signs out.
#
# Returns whether the directory was already on PATH before this ran.
function Add-QuiverToPath {
    param([string] $InstallDir)

    if (Test-OnPath -Directory $InstallDir) {
        Add-ToSessionPath -Directory $InstallDir
        return $true
    }

    $key = $null
    try {
        $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
        if (-not $key) {
            $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
        }

        $kind = [Microsoft.Win32.RegistryValueKind]::ExpandString
        try { $kind = $key.GetValueKind('Path') } catch { }

        $current = [string] $key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)

        if ([string]::IsNullOrEmpty($current)) {
            $updated = $InstallDir
        } elseif ($current.EndsWith(';')) {
            $updated = $current + $InstallDir
        } else {
            $updated = $current + ';' + $InstallDir
        }

        $key.SetValue('Path', $updated, $kind)
    } catch {
        throw "could not add $InstallDir to your PATH: $($_.Exception.Message)"
    } finally {
        if ($key) { $key.Close() }
    }

    Publish-EnvironmentChange
    Add-ToSessionPath -Directory $InstallDir
    return $false
}

function Add-ToSessionPath {
    param([string] $Directory)

    if ([string]::IsNullOrEmpty($env:Path)) {
        $env:Path = $Directory
        return
    }
    $wanted = $Directory.TrimEnd('\')
    foreach ($entry in ($env:Path -split ';')) {
        if ($entry.Trim().TrimEnd('\') -ieq $wanted) { return }
    }
    $env:Path = $env:Path.TrimEnd(';') + ';' + $Directory
}

# Publish-EnvironmentChange tells already-running processes that the environment
# block in the registry changed. Best effort throughout: if the interop type
# cannot be compiled or the broadcast fails, the install is still correct and
# the user just has to sign out instead of opening a new terminal, which the
# closing message tells them to do anyway.
function Publish-EnvironmentChange {
    try {
        if (-not ('Quiver.NativeMethods' -as [type])) {
            Add-Type -Namespace 'Quiver' -Name 'NativeMethods' -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("user32.dll", SetLastError = true, CharSet = System.Runtime.InteropServices.CharSet.Auto)]
public static extern System.IntPtr SendMessageTimeout(
    System.IntPtr hWnd, uint Msg, System.UIntPtr wParam, string lParam,
    uint fuFlags, uint uTimeout, out System.UIntPtr lpdwResult);
'@
        }
        $HWND_BROADCAST = [IntPtr] 0xffff
        $WM_SETTINGCHANGE = 0x1A
        $SMTO_ABORTIFHUNG = 0x0002
        $result = [UIntPtr]::Zero
        [Quiver.NativeMethods]::SendMessageTimeout(
            $HWND_BROADCAST, $WM_SETTINGCHANGE, [UIntPtr]::Zero, 'Environment',
            $SMTO_ABORTIFHUNG, 5000, [ref] $result) | Out-Null
    } catch {
        # Nothing to recover from: the registry write already succeeded.
    }
}

# ---------------------------------------------------------------------------
# Reporting
# ---------------------------------------------------------------------------

function Write-QuiverSuccess {
    param([string] $Target, [string] $Tag, [bool] $AlreadyOnPath)

    $installed = ''
    try {
        $installed = (& $Target --version 2>$null | Select-Object -First 1)
    } catch {
        $installed = ''
    }

    Write-Host ''
    if ([string]::IsNullOrEmpty($installed)) {
        Write-Host "Installed quiver $Tag" -ForegroundColor Green
        Write-Note "$Target did not respond to --version on this machine"
    } else {
        Write-Host "Installed $installed" -ForegroundColor Green
    }
    Write-Host "  $Target" -ForegroundColor DarkGray
    Write-Host ''

    if (-not $AlreadyOnPath) {
        Write-Host (Split-Path $Target -Parent) -ForegroundColor White -NoNewline
        Write-Host ' was added to your PATH. Open a new terminal before running quiver.'
        Write-Host ''
    }

    # Only commands the current stable release actually has. The wider command
    # tree landed after stable-26.5.1, so naming one of those here would print
    # advice that fails on the version just installed.
    Write-Host 'Next steps:'
    Write-Host '  quiver --help' -ForegroundColor White -NoNewline
    Write-Host '    everything the CLI can do'
    Write-Host '  quiver daemon' -ForegroundColor White -NoNewline
    Write-Host '    run the local daemon in the foreground'
}

# ---------------------------------------------------------------------------

# Captured before the try block: under `irm | iex` there is no script file, and
# exit would close the caller's own window rather than end a script.
$quiverInvokedAsFile = -not [string]::IsNullOrEmpty($MyInvocation.MyCommand.Path)

try {
    Install-Quiver
} catch {
    Write-Host "error: $($_.Exception.Message)" -ForegroundColor Red
    if ($quiverInvokedAsFile) {
        exit 1
    }
    $global:LASTEXITCODE = 1
}
