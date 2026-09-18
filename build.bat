@echo off
REM YunSSH build script for Windows.
REM Usage: build.bat

setlocal

set "GOEXE="

where go >nul 2>nul
if not errorlevel 1 set "GOEXE=go"

if not defined GOEXE (
    for %%P in (
        "C:\Users\%USERNAME%\.workbuddy\binaries\go\versions\1.27.1\go\bin\go.exe"
        "C:\Program Files\Go\bin\go.exe"
        "C:\Go\bin\go.exe"
    ) do (
        if exist %%P if not defined GOEXE set "GOEXE=%%~P"
    )
)

if not defined GOEXE (
    echo [x] Go toolchain not found. Install Go 1.21+ and retry.
    exit /b 1
)

echo [*] Using: %GOEXE%
echo [*] Building yssh.exe ...

"%GOEXE%" build -trimpath -o yssh.exe ./cmd/yssh
if errorlevel 1 (
    echo [x] Build failed.
    exit /b 1
)

echo [+] Built: %CD%\yssh.exe
echo.
echo Next steps:
echo   1. Add %CD% to your PATH
echo   2. yssh add web root@1.2.3.4 --port 2222 --env prod
echo   3. yssh web

endlocal
