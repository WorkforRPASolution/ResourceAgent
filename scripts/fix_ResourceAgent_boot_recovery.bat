@echo off
REM ============================================================
REM  ResourceAgent - Boot Recovery Hotfix (already-deployed PCs)
REM ============================================================
REM  Symptom : After reboot, the ResourceAgent service does not
REM            start on some PCs (kafkarest sender, Windows 7).
REM  Cause   : At boot the route to the Redis/ServiceDiscovery host
REM            is not ready yet (WSAEHOSTUNREACH, "unreachable
REM            host"). Startup fails once (fail-fast) and the
REM            service reports SERVICE_STOPPED. With the default
REM            FailureActionsFlag=0 the SCM does NOT apply the
REM            restart recovery for a graceful STOPPED, so the
REM            service stays down until the next reboot.
REM
REM  This hotfix (config only - no file copy / reinstall needed):
REM    1) (re)register failure actions: restart 5s/10s/30s/1m/5m (then 5m repeats)
REM    2) FailureActionsFlag = 1  -> recovery fires even on a
REM       graceful STOPPED with a non-zero exit code  (KEY FIX)
REM    3) start type -> Automatic (Delayed Start), to reduce the
REM       boot-time network race  (skip with /noauto)
REM    4) start the service now if it is currently stopped
REM       (skip with /nostart)
REM
REM  NOTE: Re-running install_ResourceAgent.bat resets the start
REM        type to Automatic. Re-run this hotfix afterwards, or use
REM        an updated installer that bundles these settings.
REM
REM  Usage (run as Administrator):
REM    fix_ResourceAgent_boot_recovery.bat
REM    fix_ResourceAgent_boot_recovery.bat MyServiceName
REM    fix_ResourceAgent_boot_recovery.bat /noauto
REM    fix_ResourceAgent_boot_recovery.bat /nostart
REM ============================================================

setlocal enabledelayedexpansion

REM --- Ensure core system paths are in PATH. Factory PCs sometimes
REM     have a corrupted PATH missing System32, which breaks
REM     sc/net/find/timeout. ---
set "PATH=%SystemRoot%\System32;%SystemRoot%;%SystemRoot%\System32\Wbem;%PATH%"

REM --- Defaults / argument parsing ---
set "SERVICE_NAME=ResourceAgent"
set "DO_START=1"
set "DO_DELAYED=1"

:parse
if "%~1"=="" goto :after_parse
if /i "%~1"=="/nostart" (
    set "DO_START=0"
    shift
    goto :parse
)
if /i "%~1"=="/noauto" (
    set "DO_DELAYED=0"
    shift
    goto :parse
)
set "SERVICE_NAME=%~1"
shift
goto :parse
:after_parse

echo ============================================================
echo  ResourceAgent Boot Recovery Hotfix
echo  Service: %SERVICE_NAME%
echo ============================================================
echo.

REM --- Require Administrator ---
net session >nul 2>&1
if errorlevel 1 (
    echo ERROR: Administrator privileges required.
    echo        Right-click this file and "Run as administrator".
    goto :fail
)

REM --- Service must exist ---
sc.exe query "%SERVICE_NAME%" >nul 2>&1
if errorlevel 1 (
    echo ERROR: Service "%SERVICE_NAME%" not found.
    echo        Check that ResourceAgent is installed and the name is correct.
    goto :fail
)

REM --- 1) failure actions: restart 5s / 10s / 30s (then repeats 30s) ---
echo [1/4] Registering failure actions (restart 5s/10s/30s/1m/5m)...
sc.exe failure "%SERVICE_NAME%" reset= 86400 actions= restart/5000/restart/10000/restart/30000/restart/60000/restart/300000 >nul
if errorlevel 1 (
    echo   WARNING: failed to set failure actions ^(continuing^).
) else (
    echo   OK
)

REM --- 2) FailureActionsFlag = 1  (the key fix) ---
echo [2/4] Setting FailureActionsFlag=1 (recover even on graceful STOPPED)...
sc.exe failureflag "%SERVICE_NAME%" 1 >nul
if errorlevel 1 (
    echo   WARNING: failed to set failureflag. This sc.exe may not support it.
    echo            Windows 7+ supports it. Verify: sc qfailureflag %SERVICE_NAME%
) else (
    echo   OK
)

REM --- 3) start type -> Automatic (Delayed Start) ---
if "%DO_DELAYED%"=="0" (
    echo [3/4] Skipping start-type change ^(/noauto^).
    goto :startsvc
)
echo [3/4] Setting start type to Automatic (Delayed Start)...
sc.exe config "%SERVICE_NAME%" start= delayed-auto >nul
if errorlevel 1 (
    echo   WARNING: failed to change start type ^(continuing^).
) else (
    echo   OK
)

:startsvc
REM --- 4) Start now if stopped ---
if "%DO_START%"=="0" goto :verify
echo [4/4] Checking service state...
sc.exe query "%SERVICE_NAME%" | find "RUNNING" >nul 2>&1
if not errorlevel 1 (
    echo   Already running.
    goto :verify
)
echo   Stopped - starting now...
sc.exe start "%SERVICE_NAME%" >nul 2>&1
REM Wait past the service startup grace (~5s) before checking.
timeout /t 8 /nobreak >nul
sc.exe query "%SERVICE_NAME%" | find "RUNNING" >nul 2>&1
if not errorlevel 1 (
    echo   OK: service is running.
) else (
    echo   NOTE: not RUNNING yet. If the network is not ready, the SCM
    echo         will auto-restart per the recovery policy. Check log:
    echo         ^<basePath^>\log\ResourceAgent\ResourceAgent.log
)

:verify
echo.
echo ============================================================
echo  Result
echo ============================================================
sc.exe qc "%SERVICE_NAME%" | find /i "START_TYPE"
sc.exe qfailureflag "%SERVICE_NAME%"
echo.
echo Done. From the next boot, a startup failure will auto-restart (self-heals).
echo Verify above: FAILURE_ACTIONS_FLAG should be TRUE.
echo.
endlocal
exit /b 0

:fail
echo.
echo Hotfix NOT applied.
endlocal
exit /b 1
