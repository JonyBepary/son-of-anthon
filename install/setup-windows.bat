@echo off
REM Son of Anthon - Windows Setup Script
REM Run as Administrator

echo ================================================
echo Son of Anthon - Windows Setup
echo ================================================
echo.

REM Check if running as Administrator
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo ERROR: Please run as Administrator
    pause
    exit /b 1
)

REM Get current directory
set "SCRIPT_DIR=%~dp0"
cd /d "%SCRIPT_DIR%"

REM Find the executable
if exist "son-of-anthon-windows-amd64.exe" (
    set "EXE_NAME=son-of-anthon-windows-amd64.exe"
) else if exist "son-of-anthon.exe" (
    set "EXE_NAME=son-of-anthon.exe"
) else (
    echo ERROR: No son-of-anthon executable found!
    echo Place the .exe file in this directory
    pause
    exit /b 1
)

echo Found: %EXE_NAME%

REM Create installation directory
set "INSTALL_DIR=%ProgramFiles%\SonOfAnthon"
if not exist "%INSTALL_DIR%" (
    echo Installing to %INSTALL_DIR%...
    mkdir "%INSTALL_DIR%"
)

REM Copy files
echo Copying files...
copy /Y "%EXE_NAME%" "%INSTALL_DIR%\" >nul
copy /Y "config.example.json" "%INSTALL_DIR%\config.json.example" >nul 2>nul

REM Create data directory
set "DATA_DIR=%APPDATA%\son-of-anthon"
if not exist "%DATA_DIR%" (
    echo Creating data directory...
    mkdir "%DATA_DIR%"
    mkdir "%DATA_DIR%\workspace"
)

REM Copy config if not exists
if not exist "%DATA_DIR%\config.json" (
    if exist "%INSTALL_DIR%\config.json.example" (
        echo Copy config template...
        copy /Y "%INSTALL_DIR%\config.json.example" "%DATA_DIR%\config.json"
    )
)

REM Create Start Menu shortcuts
echo Creating shortcuts...
powershell -Command "$ws = New-Object -ComObject WScript.Shell; $s = $ws.CreateShortcut('%APPDATA%\Microsoft\Windows\Start Menu\Programs\Son of Anthon.lnk'); $s.TargetPath = '%INSTALL_DIR%\son-of-anthon.exe'; $s.Arguments = 'gateway'; $s.WorkingDirectory = '%DATA_DIR%'; $s.Save()"

REM Create startup entry (optional)
echo.
echo ================================================
echo Installation complete!
echo ================================================
echo.
echo Data directory: %DATA_DIR%
echo Config file: %DATA_DIR%\config.json
echo.
echo To configure, edit: %DATA_DIR%\config.json
echo.
echo To start: son-of-anthon gateway
echo.
pause
