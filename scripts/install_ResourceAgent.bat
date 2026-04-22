@echo off
REM ResourceAgent Windows Installation Script
REM Copies files from this package to the target BasePath and registers the service.
REM Run as Administrator
REM
REM Package layout (this script must be at the root of the package):
REM   install_ResourceAgent.bat
REM   bin\x86\ResourceAgent.exe
REM   conf\ResourceAgent\{ResourceAgent,Monitor,Logging}.json
REM   utils\lhm-helper\LhmHelper.exe
REM   utils\lhm-helper\PawnIO_setup.exe
REM
REM Usage:
REM   install_ResourceAgent.bat                                    (default install, includes LhmHelper)
REM   install_ResourceAgent.bat /basepath D:\EARS\EEGAgent         (specify basepath)
REM   install_ResourceAgent.bat /nolhm                             (exclude LhmHelper + PawnIO)
REM   install_ResourceAgent.bat /site 10.0.0.1,10.0.0.2            (set VirtualAddressList directly)
REM   install_ResourceAgent.bat /nocopy                            (skip file copy, register service only)
REM   install_ResourceAgent.bat /uninstall                         (uninstall)

setlocal enabledelayedexpansion

REM --- Ensure core system paths are in PATH ---
REM Factory PCs sometimes have a corrupted/overwritten PATH env var that is
REM missing System32. Without this, net/sc/reg/xcopy/fsutil/timeout all fail
REM with "command not found", which previously surfaced as a false
REM "requires Administrator privileges" error.
set "PATH=%SystemRoot%\System32;%SystemRoot%;%SystemRoot%\System32\Wbem;%PATH%"

REM --- Package directory = where this script lives ---
set "PKG_DIR=%~dp0"

REM --- Default values ---
set "BASE_PATH=D:\EARS\EEGAgent"
set "BASEPATH_SET=0"
set "INCLUDE_LHM=1"
set "NO_COPY=0"
set "UNINSTALL=0"
set "SITE_ADDR_ARG="
set "SERVICE_NAME=ResourceAgent"
set "DISPLAY_NAME=Resource Monitoring Service"
set "DESCRIPTION=Lightweight monitoring agent for collecting hardware resource metrics"
set "NDP_RELEASE=0"
set "NDP_WARNED=0"

REM --- Parse arguments ---
:parse_args
if "%~1"=="" goto :args_done
if /i "%~1"=="/basepath" (
    set "BASE_PATH=%~2"
    set "BASEPATH_SET=1"
    shift
    shift
    goto :parse_args
)
if /i "%~1"=="/nolhm" (
    set "INCLUDE_LHM=0"
    shift
    goto :parse_args
)
if /i "%~1"=="/nocopy" (
    set "NO_COPY=1"
    shift
    goto :parse_args
)
if /i "%~1"=="/site" (
    set "SITE_ADDR_ARG=%~2"
    shift
    shift
    goto :parse_args
)
if /i "%~1"=="/uninstall" (
    set "UNINSTALL=1"
    shift
    goto :parse_args
)
echo Unknown option: %~1
echo Usage: %~nx0 [/basepath PATH] [/nolhm] [/nocopy] [/site ADDR] [/uninstall]
exit /b 1
:args_done

REM --- Target paths ---
set "BIN_DIR=%BASE_PATH%\bin\x86"
set "CONF_DIR=%BASE_PATH%\conf\ResourceAgent"
set "LOG_DIR=%BASE_PATH%\log\ResourceAgent"
set "TOOLS_DIR=%BASE_PATH%\utils\lhm-helper"

REM --- Check admin privileges ---
net session >nul 2>&1
if errorlevel 1 (
    echo ERROR: This script requires Administrator privileges.
    echo Right-click and select "Run as administrator".
    exit /b 1
)

if "%UNINSTALL%"=="1" (
    goto :uninstall
) else (
    goto :install
)

REM =============================================================
REM  INSTALL
REM =============================================================
:install
REM --- Detect BASE_PATH from ARSAgent service if not explicitly specified ---
if "!BASEPATH_SET!"=="1" goto :install_start
sc.exe query ARSAgent >nul 2>&1
if errorlevel 1 (
    echo ERROR: /basepath not specified and ARSAgent service not found.
    echo        Usage: %~nx0 /basepath D:\EARS\EEGAgent
    exit /b 1
)
for /f "delims=" %%L in ('sc.exe qc ARSAgent ^| findstr /i "BINARY_PATH_NAME"') do set "SVC_LINE=%%L"
set "SVC_BIN_RAW=!SVC_LINE:*: =!"
set "SVC_BIN_RAW=!SVC_BIN_RAW:"=!"
for /f "tokens=1" %%P in ("!SVC_BIN_RAW!") do set "SVC_EXE=%%P"
for %%F in ("!SVC_EXE!") do set "SVC_DIR=%%~dpF"
set "SVC_DIR=!SVC_DIR:~0,-1!"
for %%D in ("!SVC_DIR!") do set "SVC_DIR=%%~dpD"
set "SVC_DIR=!SVC_DIR:~0,-1!"
for %%D in ("!SVC_DIR!") do set "BASE_PATH=%%~dpD"
set "BASE_PATH=!BASE_PATH:~0,-1!"
echo   Detected basepath from ARSAgent service: !BASE_PATH!
set "BIN_DIR=!BASE_PATH!\bin\x86"
set "CONF_DIR=!BASE_PATH!\conf\ResourceAgent"
set "LOG_DIR=!BASE_PATH!\log\ResourceAgent"
set "TOOLS_DIR=!BASE_PATH!\utils\lhm-helper"

:install_start
echo Installing ResourceAgent...
echo   Target:  %BASE_PATH%
if "%NO_COPY%"=="1" (
    echo   Mode:    Service registration only (file copy skipped)
) else (
    echo   Package: %PKG_DIR%
)
echo.

REM Create target directory structure
for %%D in ("%BIN_DIR%" "%CONF_DIR%" "%LOG_DIR%") do (
    if not exist %%D (
        mkdir %%D
        echo   Created: %%~D
    )
)

if "%NO_COPY%"=="1" goto :skip_file_copy

REM --- Copy ResourceAgent.exe ---
if not exist "%PKG_DIR%bin\x86\ResourceAgent.exe" (
    echo ERROR: bin\x86\ResourceAgent.exe not found in package.
    exit /b 1
)
copy /y "%PKG_DIR%bin\x86\ResourceAgent.exe" "%BIN_DIR%\ResourceAgent.exe" >nul
echo   Copied ResourceAgent.exe

REM --- Copy config files (skip if already exist at target) ---
for %%F in (ResourceAgent.json Monitor.json Logging.json) do (
    if not exist "%PKG_DIR%conf\ResourceAgent\%%F" (
        echo ERROR: conf\ResourceAgent\%%F not found in package.
        exit /b 1
    )
    if not exist "%CONF_DIR%\%%F" (
        copy /y "%PKG_DIR%conf\ResourceAgent\%%F" "%CONF_DIR%\%%F" >nul
        echo   Copied %%F
    ) else (
        echo   Skipped %%F (already exists at target)
    )
)

goto :site_selection

:skip_file_copy
REM --- /nocopy: verify required files exist at target ---
if not exist "%BIN_DIR%\ResourceAgent.exe" (
    echo ERROR: %BIN_DIR%\ResourceAgent.exe not found. Copy files before using /nocopy.
    exit /b 1
)
if not exist "%CONF_DIR%\ResourceAgent.json" (
    echo ERROR: %CONF_DIR%\ResourceAgent.json not found. Copy files before using /nocopy.
    exit /b 1
)
echo   Verified: ResourceAgent.exe and config files exist

:site_selection
REM --- Site selection: configure VirtualAddressList ---
if defined SITE_ADDR_ARG (
    REM /site <address> — set VirtualAddressList directly
    set "SITE_ADDR=!SITE_ADDR_ARG!"
    goto :apply_site
)

REM Interactive mode via sites.conf
set "SITES_FILE=%PKG_DIR%sites.conf"
if not exist "%SITES_FILE%" goto :site_done
set "SITE_COUNT=0"
for /f "usebackq eol=# tokens=1,* delims==" %%A in ("%SITES_FILE%") do (
    set "%%A=%%B"
)
if !SITE_COUNT! EQU 0 goto :site_done

echo.
echo === Site Selection ===
for /L %%I in (1,1,!SITE_COUNT!) do (
    call set "MENU_NAME=%%SITE_%%I_NAME%%"
    call set "MENU_ADDR=%%SITE_%%I_ADDR%%"
    echo   %%I^) !MENU_NAME! ^(!MENU_ADDR!^)
)
echo   0^) Skip ^(do not modify VirtualAddressList^)
echo.
set /p "SELECTED_SITE=Select site [0-!SITE_COUNT!]: "

if "!SELECTED_SITE!"=="0" (
    echo   Site selection skipped
    goto :site_done
)
if !SELECTED_SITE! GTR !SITE_COUNT! (
    echo ERROR: Invalid site number: !SELECTED_SITE!
    exit /b 1
)
call set "SITE_ADDR=%%SITE_!SELECTED_SITE!_ADDR%%"
call set "SITE_NAME_SEL=%%SITE_!SELECTED_SITE!_NAME%%"
if not defined SITE_ADDR (
    echo ERROR: Invalid site number: !SELECTED_SITE!
    exit /b 1
)

:apply_site
REM Update VirtualAddressList in ResourceAgent.json
set "RA_CONFIG=%CONF_DIR%\ResourceAgent.json"
if exist "!RA_CONFIG!" (
    set "TEMP_CONFIG=!RA_CONFIG!.tmp"
    (for /f "usebackq delims=" %%L in ("!RA_CONFIG!") do (
        set "LINE=%%L"
        if "!LINE:VirtualAddressList=!" neq "!LINE!" (
            echo   "VirtualAddressList": "!SITE_ADDR!",
        ) else (
            echo(!LINE!
        )
    )) > "!TEMP_CONFIG!"
    move /y "!TEMP_CONFIG!" "!RA_CONFIG!" >nul
    if defined SITE_NAME_SEL (
        echo   VirtualAddressList set to: !SITE_ADDR! ^(!SITE_NAME_SEL!^)
    ) else (
        echo   VirtualAddressList set to: !SITE_ADDR!
    )
)
:site_done

if "%NO_COPY%"=="1" goto :nocopy_lhm

REM --- Check .NET Framework 4.8+ for LhmHelper (warn only, no auto-install) ---
REM LhmHelper targets .NET Framework 4.7.2 (works with 4.8+ runtime).
REM Factory equipment PCs must not have system-level installs triggered automatically;
REM administrator must install .NET Framework 4.8 manually if needed.
if "%INCLUDE_LHM%"=="1" (
    set "NDP_RELEASE=0"
    for /f "tokens=3" %%A in ('reg query "HKLM\SOFTWARE\Microsoft\NET Framework Setup\NDP\v4\Full" /v Release 2^>nul ^| findstr /i "Release"') do (
        set "NDP_RELEASE=%%A"
    )

    set "NDP_OK=0"
    if !NDP_RELEASE! GEQ 528040 set "NDP_OK=1"

    if "!NDP_OK!"=="1" (
        echo   .NET Framework 4.8+ detected ^(Release: !NDP_RELEASE!^).
    ) else (
        echo.
        echo ===============================================================
        echo  WARNING: .NET Framework 4.8+ NOT DETECTED
        echo    Current Release: !NDP_RELEASE!  ^(required: 528040+^)
        echo.
        echo  LhmHelper ^(hardware sensor collection^) requires .NET 4.8+.
        echo  ResourceAgent will be installed, but LhmHelper will FAIL to start.
        echo  Hardware sensors ^(temperature, fan, GPU, voltage, S.M.A.R.T^)
        echo  will return EMPTY data. OS metrics ^(CPU/Memory/Disk/Network^)
        echo  will continue to work normally.
        echo.
        echo  TO ENABLE HARDWARE MONITORING:
        echo    1. Contact system administrator
        echo    2. Install .NET Framework 4.8 offline installer:
        echo       https://dotnet.microsoft.com/download/dotnet-framework/net48
        echo    3. Reboot the PC
        echo    4. Restart ResourceAgent service:
        echo       sc.exe stop ResourceAgent
        echo       sc.exe start ResourceAgent
        echo ===============================================================
        echo.
        set "NDP_WARNED=1"
    )
)

REM --- Copy LhmHelper + PawnIO (optional) ---
if "%INCLUDE_LHM%"=="1" (
    if not exist "%TOOLS_DIR%" mkdir "%TOOLS_DIR%"

    REM Copy LhmHelper directory (exe + config + dependency DLLs)
    if not exist "%PKG_DIR%utils\lhm-helper\LhmHelper.exe" (
        echo ERROR: utils\lhm-helper\LhmHelper.exe not found in package.
        echo        Rebuild package with: package.sh --lhmhelper or use /nolhm to skip
        exit /b 1
    )
    REM Exclude .NET Framework installer from being copied to target (~112MB, only needed during install).
    xcopy /y /i "%PKG_DIR%utils\lhm-helper\LhmHelper.exe" "%TOOLS_DIR%\" >nul
    if exist "%PKG_DIR%utils\lhm-helper\LhmHelper.exe.config" (
        xcopy /y /i "%PKG_DIR%utils\lhm-helper\LhmHelper.exe.config" "%TOOLS_DIR%\" >nul
    )
    xcopy /y /i "%PKG_DIR%utils\lhm-helper\*.dll" "%TOOLS_DIR%\" >nul 2>&1
    echo   Copied LhmHelper and dependency DLLs

    REM Detect OS version to determine PawnIO compatibility
    REM PawnIO requires Windows 8+ (version 6.2+). Windows 7 = 6.1.
    REM On Windows 7, LHM automatically falls back to WinRing0 (embedded).
    set "SKIP_PAWNIO=0"
    for /f "tokens=4 delims=. " %%A in ('ver') do set "WIN_VER=%%A"
    for /f "tokens=4,5 delims=. " %%A in ('ver') do (
        set "WIN_MAJOR=%%A"
        set "WIN_MINOR=%%B"
    )
    if defined WIN_MAJOR if defined WIN_MINOR (
        if !WIN_MAJOR! LEQ 6 if !WIN_MINOR! LEQ 1 set "SKIP_PAWNIO=1"
    )

    if "!SKIP_PAWNIO!"=="1" (
        echo   Windows 7 detected: skipping PawnIO driver ^(LHM will use WinRing0 fallback^)
    ) else (
        REM Copy PawnIO_setup.exe
        if not exist "%PKG_DIR%utils\lhm-helper\PawnIO_setup.exe" (
            echo ERROR: utils\lhm-helper\PawnIO_setup.exe not found in package.
            exit /b 1
        )
        copy /y "%PKG_DIR%utils\lhm-helper\PawnIO_setup.exe" "%TOOLS_DIR%\PawnIO_setup.exe" >nul
        echo   Copied PawnIO_setup.exe

        REM Install PawnIO driver if not already installed
        echo   Checking PawnIO driver...
        sc.exe query PawnIO >nul 2>&1
        if errorlevel 1 (
            echo   PawnIO driver not installed. Installing...
            "%TOOLS_DIR%\PawnIO_setup.exe" -install -silent
            if errorlevel 1 (
                echo ERROR: PawnIO driver installation failed.
                exit /b 1
            )
            echo   PawnIO driver installed successfully
        ) else (
            echo   PawnIO driver already installed, skipping
        )
    )
)
goto :register_service

:nocopy_lhm
REM --- /nocopy: install PawnIO driver if LhmHelper exists at target ---
if "%INCLUDE_LHM%"=="1" (
    if exist "%TOOLS_DIR%\LhmHelper.exe" (
        echo   LhmHelper.exe found at target

        set "SKIP_PAWNIO=0"
        for /f "tokens=4,5 delims=. " %%A in ('ver') do (
            set "WIN_MAJOR=%%A"
            set "WIN_MINOR=%%B"
        )
        if defined WIN_MAJOR if defined WIN_MINOR (
            if !WIN_MAJOR! LEQ 6 if !WIN_MINOR! LEQ 1 set "SKIP_PAWNIO=1"
        )

        if "!SKIP_PAWNIO!"=="1" (
            echo   Windows 7 detected: skipping PawnIO driver ^(LHM will use WinRing0 fallback^)
        ) else (
            if exist "%TOOLS_DIR%\PawnIO_setup.exe" (
                sc.exe query PawnIO >nul 2>&1
                if errorlevel 1 (
                    echo   PawnIO driver not installed. Installing...
                    "%TOOLS_DIR%\PawnIO_setup.exe" -install -silent
                    if errorlevel 1 (
                        echo ERROR: PawnIO driver installation failed.
                        exit /b 1
                    )
                    echo   PawnIO driver installed successfully
                ) else (
                    echo   PawnIO driver already installed, skipping
                )
            )
        )
    )
)

:register_service
REM --- Register Windows service ---
set "BINARY_PATH=%BIN_DIR%\ResourceAgent.exe"
set "CONFIG_FILE=%CONF_DIR%\ResourceAgent.json"
set "MONITOR_FILE=%CONF_DIR%\Monitor.json"
set "LOGGING_FILE=%CONF_DIR%\Logging.json"
set "SERVICE_PATH=\"%BINARY_PATH%\" -config \"%CONFIG_FILE%\" -monitor \"%MONITOR_FILE%\" -logging \"%LOGGING_FILE%\""

REM Remove existing service if present
sc.exe query %SERVICE_NAME% >nul 2>&1
if not errorlevel 1 (
    echo   Stopping existing service...
    sc.exe stop %SERVICE_NAME% >nul 2>&1
    timeout /t 2 /nobreak >nul
    sc.exe delete %SERVICE_NAME% >nul 2>&1
    timeout /t 2 /nobreak >nul
)

echo   Creating Windows service...
sc.exe create %SERVICE_NAME% binPath= "%SERVICE_PATH%" start= auto DisplayName= "%DISPLAY_NAME%" >nul
sc.exe description %SERVICE_NAME% "%DESCRIPTION%" >nul
sc.exe failure %SERVICE_NAME% reset= 86400 actions= restart/5000/restart/10000/restart/30000 >nul

REM Record install result for ResourceAgent to report to Kafka on first run.
if not exist "%LOG_DIR%" mkdir "%LOG_DIR%"
(
    echo install_timestamp=!date! !time!
    echo ndp_release=!NDP_RELEASE!
    echo ndp_warned=!NDP_WARNED!
) > "%LOG_DIR%\install_result.txt"

echo   Starting service...
sc.exe start %SERVICE_NAME% >nul 2>&1

REM Verify
timeout /t 2 /nobreak >nul
sc.exe query %SERVICE_NAME% | find "RUNNING" >nul 2>&1
if not errorlevel 1 (
    echo.
    echo ResourceAgent installed and running successfully!
    echo   BasePath: %BASE_PATH%
    echo   Binary:   %BIN_DIR%\ResourceAgent.exe
    echo   Config:   %CONF_DIR%\
    echo   Logs:     %LOG_DIR%\
    if "!NDP_WARNED!"=="1" (
        echo.
        echo   NOTE: LhmHelper will fail until .NET Framework 4.8 is installed.
        echo         Hardware sensors ^(temperature/fan/GPU/S.M.A.R.T^) unavailable.
    )
) else (
    echo WARNING: Service installed but not running. Check logs for details.
)
goto :eof

REM =============================================================
REM  UNINSTALL
REM =============================================================
:uninstall
echo Uninstalling ResourceAgent...

REM --- Detect installed BASE_PATH from service if not explicitly specified ---
if "!BASEPATH_SET!"=="1" goto :uninstall_start
sc.exe query %SERVICE_NAME% >nul 2>&1
if errorlevel 1 goto :uninstall_start
for /f "delims=" %%L in ('sc.exe qc %SERVICE_NAME% ^| findstr /i "BINARY_PATH_NAME"') do set "SVC_LINE=%%L"
set "SVC_BIN_RAW=!SVC_LINE:*: =!"
set "SVC_BIN_RAW=!SVC_BIN_RAW:"=!"
for /f "tokens=1" %%P in ("!SVC_BIN_RAW!") do set "SVC_EXE=%%P"
for %%F in ("!SVC_EXE!") do set "SVC_DIR=%%~dpF"
set "SVC_DIR=!SVC_DIR:~0,-1!"
for %%D in ("!SVC_DIR!") do set "SVC_DIR=%%~dpD"
set "SVC_DIR=!SVC_DIR:~0,-1!"
for %%D in ("!SVC_DIR!") do set "DETECTED_BASE=%%~dpD"
set "DETECTED_BASE=!DETECTED_BASE:~0,-1!"
if defined DETECTED_BASE (
    echo   Detected install path from service: !DETECTED_BASE!
    set "BASE_PATH=!DETECTED_BASE!"
    set "BIN_DIR=!BASE_PATH!\bin\x86"
    set "CONF_DIR=!BASE_PATH!\conf\ResourceAgent"
    set "LOG_DIR=!BASE_PATH!\log\ResourceAgent"
    set "TOOLS_DIR=!BASE_PATH!\utils\lhm-helper"
)

:uninstall_start

REM Stop and remove service
sc.exe query %SERVICE_NAME% >nul 2>&1
if not errorlevel 1 (
    echo   Stopping service...
    sc.exe stop %SERVICE_NAME% >nul 2>&1
    timeout /t 2 /nobreak >nul
    sc.exe delete %SERVICE_NAME% >nul 2>&1
    echo   Service removed
)

REM Uninstall PawnIO driver if installed
sc.exe query PawnIO >nul 2>&1
if not errorlevel 1 (
    if exist "%TOOLS_DIR%\PawnIO_setup.exe" (
        echo   Uninstalling PawnIO driver...
        "%TOOLS_DIR%\PawnIO_setup.exe" -uninstall -silent
        echo   PawnIO driver uninstalled
    ) else (
        echo   WARNING: PawnIO driver is installed but PawnIO_setup.exe not found.
        echo            Uninstall PawnIO manually from Control Panel.
    )
)

REM Confirm file removal
set /p "CONFIRM=Remove ResourceAgent files from %BASE_PATH%? (y/N): "
if /i not "%CONFIRM%"=="y" (
    echo   Skipped file removal.
    echo ResourceAgent service uninstalled.
    goto :eof
)

REM Remove ResourceAgent files only (preserve ARSAgent)
if exist "%BIN_DIR%\ResourceAgent.exe" del /f "%BIN_DIR%\ResourceAgent.exe"
if exist "%CONF_DIR%" rmdir /s /q "%CONF_DIR%"
if exist "%LOG_DIR%" rmdir /s /q "%LOG_DIR%"
if exist "%TOOLS_DIR%" rmdir /s /q "%TOOLS_DIR%"
echo   ResourceAgent files removed (ARSAgent files preserved)

echo ResourceAgent uninstalled successfully!
goto :eof
