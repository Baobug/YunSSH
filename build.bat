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

set CGO_ENABLED=0

echo [*] Using: %GOEXE%
echo.
echo [*] Building yssh.exe ...
"%GOEXE%" build -trimpath -ldflags "-s -w" -o yssh.exe ./cmd/yssh
if errorlevel 1 (
    echo [x] Build failed: yssh
    exit /b 1
)

echo [*] Building ysshtray.exe ...
"%GOEXE%" build -trimpath -ldflags "-s -w -H=windowsgui" -o ysshtray.exe ./cmd/ysshtray
if errorlevel 1 (
    echo [x] Build failed: ysshtray
    exit /b 1
)

echo.
echo [+] Built:
echo       %CD%\yssh.exe
echo       %CD%\ysshtray.exe
echo.
echo Next steps:
echo   yssh install         Install to %%LOCALAPPDATA%%\Programs\YunSSH
echo   yssh add web root@1.2.3.4 --port 2222 --env prod
echo   yssh web
echo.
echo Note: ysshtray.exe is a GUI binary; double-clicking it starts the tray.

endlocal
