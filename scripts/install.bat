@echo off
REM ResourceAgent Windows Installation Script
REM Copies files from this package to the target BasePath and registers the service.
REM Run as Administrator
REM
REM Package layout (this script must be at the root of the package):
REM   install.bat
REM   bin\x86\ResourceAgent.exe
REM   conf\ResourceAgent\{ResourceAgent,Monitor,Logging}.json
REM   utils\lhm-helper\LhmHelper.exe        (optional)
REM   utils\lhm-helper\PawnIO_setup.exe      (optional)
REM
REM Usage:
REM   install.bat                                    (default install)
REM   install.bat /basepath D:\EARS\EEGAgent         (specify basepath)
REM   install.bat /lhmhelper                         (include LhmHelper + PawnIO)
REM   install.bat /site 1                            (non-interactive site selection)
REM   install.bat /uninstall                         (uninstall)

setlocal enabledelayedexpansion

REM --- Package directory = where this script lives ---
set "PKG_DIR=%~dp0"

REM --- Default values ---
set "BASE_PATH=D:\EARS\EEGAgent"
set "INCLUDE_LHM=0"
set "UNINSTALL=0"
set "SITE_NUM="
set "SERVICE_NAME=ResourceAgent"
set "DISPLAY_NAME=ResourceAgent Monitoring Service"
set "DESCRIPTION=Lightweight monitoring agent for collecting hardware resource metrics"

REM --- Parse arguments ---
:parse_args
if "%~1"=="" goto :args_done
if /i "%~1"=="/basepath" (
    set "BASE_PATH=%~2"
    shift
    shift
    goto :parse_args
)
if /i "%~1"=="/lhmhelper" (
    set "INCLUDE_LHM=1"
    shift
    goto :parse_args
)
if /i "%~1"=="/site" (
    set "SITE_NUM=%~2"
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
echo Usage: %~nx0 [/basepath PATH] [/lhmhelper] [/site N] [/uninstall]
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
echo Installing ResourceAgent...
echo   Package: %PKG_DIR%
echo   Target:  %BASE_PATH%
echo.

REM Create target directory structure
for %%D in ("%BIN_DIR%" "%CONF_DIR%" "%LOG_DIR%") do (
    if not exist %%D (
        mkdir %%D
        echo   Created: %%~D
    )
)

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

REM --- Site selection: configure VirtualAddressList ---
set "SITES_FILE=%PKG_DIR%sites.conf"
if exist "%SITES_FILE%" (
    REM Parse sites.conf
    set "SITE_COUNT=0"
    for /f "usebackq eol=# tokens=1,* delims==" %%A in ("%SITES_FILE%") do (
        set "%%A=%%B"
    )
    if !SITE_COUNT! GTR 0 (
        if defined SITE_NUM (
            REM Non-interactive mode: /site N
            if "!SITE_NUM!"=="0" (
                echo   Site selection skipped ^(/site 0^)
                goto :site_done
            )
            if !SITE_NUM! GTR !SITE_COUNT! (
                echo ERROR: /site !SITE_NUM! is out of range ^(1-!SITE_COUNT!^)
                exit /b 1
            )
            set "SELECTED_SITE=!SITE_NUM!"
        ) else (
            REM Interactive mode: show menu
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
        )
        if "!SELECTED_SITE!"=="0" (
            echo   Site selection skipped
            goto :site_done
        )
        REM Validate selection and resolve indirection
        call set "SITE_ADDR=%%SITE_!SELECTED_SITE!_ADDR%%"
        call set "SITE_NAME_SEL=%%SITE_!SELECTED_SITE!_NAME%%"
        if not defined SITE_ADDR (
            echo ERROR: Invalid site number: !SELECTED_SITE!
            exit /b 1
        )
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
            echo   VirtualAddressList set to: !SITE_ADDR! ^(!SITE_NAME_SEL!^)
        )
    )
)
:site_done

REM --- Copy LhmHelper + PawnIO (optional) ---
if "%INCLUDE_LHM%"=="1" (
    if not exist "%TOOLS_DIR%" mkdir "%TOOLS_DIR%"

    REM Copy LhmHelper.exe
    if not exist "%PKG_DIR%utils\lhm-helper\LhmHelper.exe" (
        echo ERROR: utils\lhm-helper\LhmHelper.exe not found in package.
        echo        Rebuild package with: package.sh --lhmhelper
        exit /b 1
    )
    copy /y "%PKG_DIR%utils\lhm-helper\LhmHelper.exe" "%TOOLS_DIR%\LhmHelper.exe" >nul
    echo   Copied LhmHelper.exe

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
) else (
    echo WARNING: Service installed but not running. Check logs for details.
)
goto :eof

REM =============================================================
REM  UNINSTALL
REM =============================================================
:uninstall
echo Uninstalling ResourceAgent...

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
