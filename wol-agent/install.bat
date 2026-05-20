@echo off
setlocal
set "AGENT_DIR=%~dp0"
set "AGENT_EXE=%AGENT_DIR%wol-agent.exe"

if not exist "%AGENT_EXE%" (
    echo [ERROR] wol-agent.exe not found. Please build it first:
    echo   go build -ldflags="-s -w" -o wol-agent.exe
    pause
    exit /b 1
)

where nssm >nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] nssm not found in PATH.
    echo Download nssm from: https://nssm.cc/download
    echo Place nssm.exe in a PATH directory or in this folder.
    pause
    exit /b 1
)

echo Installing wol-agent as Windows Service...
nssm stop WolAgent >nul 2>&1
nssm remove WolAgent confirm >nul 2>&1
nssm install WolAgent "%AGENT_EXE%" --port 32249
nssm set WolAgent AppDirectory "%AGENT_DIR%"
nssm set WolAgent Start SERVICE_AUTO_START
nssm set WolAgent Description "Wake-on-LAN Agent - remote status check and shutdown"
nssm start WolAgent
echo.
echo Service installed and started successfully.
pause
