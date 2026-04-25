@echo off
REM OS bitness verification script for ResourceAgent deployment.
REM Compares an expected bitness (32 or 64) against the actual host OS bitness.
REM
REM Designed to be invoked via WebManager's exec task, where success/failure is
REM determined by the exit code (stdout is not shown in the UI).
REM
REM Usage:
REM   check_osbits.bat 32    (expect 32-bit OS)
REM   check_osbits.bat 64    (expect 64-bit OS)
REM
REM Output contract:
REM   Match        : stdout "true",  exit 0  -> WebManager shows success
REM   Mismatch     : stdout "false", exit 1  -> WebManager shows failure (Exit value: 1)
REM   Invalid args : stdout usage,   exit 2  -> WebManager shows failure (Exit value: 2)
REM
REM Detection logic (works from Windows 7 SP1 32-bit and up, even under WOW64):
REM   if PROCESSOR_ARCHITEW6432 is defined, we are a 32-bit process on a 64-bit OS
REM   else PROCESSOR_ARCHITECTURE == x86 means 32-bit OS, anything else means 64-bit.

setlocal

if "%~1"=="" goto :usage
if "%~1"=="32" goto :args_ok
if "%~1"=="64" goto :args_ok
goto :usage

:args_ok
set "EXPECTED=%~1"

if defined PROCESSOR_ARCHITEW6432 (
    set "ACTUAL=64"
) else if /i "%PROCESSOR_ARCHITECTURE%"=="x86" (
    set "ACTUAL=32"
) else (
    set "ACTUAL=64"
)

if "%EXPECTED%"=="%ACTUAL%" (
    echo true
    endlocal
    exit /b 0
) else (
    echo false
    endlocal
    exit /b 1
)

:usage
echo usage: check_osbits.bat [32^|64]
endlocal
exit /b 2
