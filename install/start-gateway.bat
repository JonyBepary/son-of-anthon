@echo off
REM Son of Anthon - Windows Gateway Launcher
REM Auto-start on Windows boot

set "EXE_DIR=%~dp0"
cd /d "%EXE_DIR%"

REM Set config path
set "PERSONAL_OS_CONFIG=%APPDATA%\son-of-anthon\config.json"

REM Start the gateway
start /B "" son-of-anthon.exe gateway
