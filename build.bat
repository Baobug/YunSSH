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

REM 版本号取仓库里版本号最大的 tag。这里刻意不用 git describe：本项目的
REM 发布 tag 都打在 main 的合并提交上，从 dev 出发没有可达的 tag，describe
REM 会直接失败。取不到任何 tag 就回退到 0.0.0，让临时构建在
REM 「属性 - 详细信息」里一眼看出并非正式版本。
set "VERSION="
for /f "delims=" %%i in ('git tag --list "v*" --sort=-v:refname 2^>nul') do (
    if not defined VERSION set "VERSION=%%i"
)
if not defined VERSION set "VERSION=v0.0.0"
set "VERSION=%VERSION:v=%"

REM 版本号要同时写进 exe 的版本资源（由 genicon 生成，见 internal/appicon）
REM 和程序自身的输出，所以 go run 与 go build 都得注入，否则两处会不一致。
set "LDFLAGS=-s -w -X github.com/Baobug/YunSSH.Version=%VERSION%"

echo [*] Using: %GOEXE%
echo [*] Version: %VERSION%
echo.
echo [*] Generating icon and version resources ...
"%GOEXE%" run -ldflags "%LDFLAGS%" ./tools/genicon
if errorlevel 1 (
    echo [x] Build failed: resources
    exit /b 1
)
echo.

echo [*] Building yssh.exe ...
"%GOEXE%" build -trimpath -ldflags "%LDFLAGS%" -o yssh.exe ./cmd/yssh
if errorlevel 1 (
    echo [x] Build failed: yssh
    exit /b 1
)

echo [*] Building ysshtray.exe ...
"%GOEXE%" build -trimpath -ldflags "%LDFLAGS% -H=windowsgui" -o ysshtray.exe ./cmd/ysshtray
if errorlevel 1 (
    echo [x] Build failed: ysshtray
    exit /b 1
)

echo.
echo [+] Built:
echo       %CD%\yssh.exe
echo       %CD%\ysshtray.exe
echo.
echo Note: both exe files carry the application icon and the version resource
echo       (copyright, product name, version). Both are produced at build time
echo       by tools/genicon, so the repository holds no binary assets.
echo.
echo Next steps:
echo   yssh install         Install to %%LOCALAPPDATA%%\Programs\YunSSH
echo   yssh add web root@1.2.3.4 --port 2222 --env prod
echo   yssh web
echo.
echo Note: ysshtray.exe is a GUI binary; double-clicking it starts the tray.

endlocal
