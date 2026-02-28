@echo off
setlocal enabledelayedexpansion

set "APP_NAME=son-of-anthon"
set "INSTALL_DIR=%ProgramFiles%\%APP_NAME%"
set "CONFIG_DIR=%APPDATA%\%APP_NAME%"
set "BINARY_NAME=son-of-anthon-windows-amd64.exe"

title Son of Anthon - Windows Installer

:: Check admin
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo This installer requires Administrator privileges.
    echo Please right-click and select "Run as administrator"
    pause
    exit /b 1
)

echo ============================================
echo  Son of Anthon - Windows Installer
echo ============================================
echo.

:: Find binary
if exist "%~dp0son-of-anthon-windows-amd64.exe" (
    set "SOURCE=%~dp0son-of-anthon-windows-amd64.exe"
) else if exist "%~dp0son-of-anthon.exe" (
    set "SOURCE=%~dp0son-of-anthon.exe"
) else (
    echo ERROR: No son-of-anthon executable found!
    echo Place the .exe file in this directory
    pause
    exit /b 1
)

echo Found: %SOURCE%
echo.

:: Create directories
echo Creating directories...
if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%"
if not exist "%CONFIG_DIR%" mkdir "%CONFIG_DIR%"
if not exist "%CONFIG_DIR%\workspace" mkdir "%CONFIG_DIR%\workspace"

:: Copy binary
echo Installing binary...
copy /Y "%SOURCE%" "%INSTALL_DIR%\" >nul

:: Copy config example
if not exist "%CONFIG_DIR%\config.json" (
    if exist "%~dp0config.example.json" (
        copy /Y "%~dp0config.example.json" "%CONFIG_DIR%\config.json"
        echo Created config from template - please edit with your API keys
    )
)

:: Create uninstaller
echo Creating uninstaller...
(
echo @echo off
echo del /q "%INSTALL_DIR%\*"
echo rmdir "%INSTALL_DIR%"
echo del /q "%CONFIG_DIR%\config.json" 2^>nul
echo echo Uninstalled
echo pause
) > "%INSTALL_DIR%\uninstall.bat"

:: Install NSSM (Non-Sucking Service Manager) for Windows service
where nssm >nul 2>&1
if %errorLevel% neq 0 (
    echo Installing NSSM for Windows service...
    powershell -Command "Invoke-WebRequest -Uri 'https://nssm.cc/release/nssm-2.24.zip' -OutFile '%TEMP%\nssm.zip'"
    powershell -Command "Expand-Archive -Path '%TEMP%\nssm.zip' -DestinationPath '%TEMP%\nssm' -Force"
    copy /Y "%TEMP%\nssm\nssm-2.24\win64\nssm.exe" "%INSTALL_DIR%\" >nul
    del /q "%TEMP%\nssm.zip"
    rmdir /s /q "%TEMP%\nssm"
)

:: Install Windows service
echo Installing Windows service...
"%INSTALL_DIR%\nssm.exe" install %APP_NAME% "%INSTALL_DIR%\%BINARY_NAME%" gateway
"%INSTALL_DIR%\nssm.exe" set %APP_NAME% AppDirectory "%CONFIG_DIR%"
"%INSTALL_DIR%\nssm.exe" set %APP_NAME% AppEnvironmentExtra "HOME=%USERPROFILE%,PERSONAL_OS_CONFIG=%APPDATA%\son-of-anthon\config.json"
"%INSTALL_DIR%\nssm.exe" set %APP_NAME% Description "Son of Anthon - Multi-agent AI Assistant"
"%INSTALL_DIR%\nssm.exe" set %APP_NAME% Start SERVICE_AUTO_START

:: Create Start Menu shortcuts
echo Creating shortcuts...
powershell -Command "$ws = New-Object -ComObject WScript.Shell; $s = $ws.CreateShortcut('%APPDATA%\Microsoft\Windows\Start Menu\Programs\Son of Anthon.lnk'); $s.TargetPath = '%INSTALL_DIR%\%BINARY_NAME%'; $s.Arguments = 'gateway'; $s.WorkingDirectory = '%CONFIG_DIR%'; $s.Save()"

:: Start service
echo Starting service...
net start %APP_NAME% >nul 2>&1

echo.
echo ============================================
echo  Installation Complete!
echo ============================================
echo.
echo Data directory: %CONFIG_DIR%
echo Config file: %CONFIG_DIR%\config.json
echo.
echo To edit config, run:
echo   notepad %CONFIG_DIR%\config.json
echo.
echo To manage service:
echo   net start %APP_NAME%
echo   net stop %APP_NAME%
echo.
echo To uninstall:
echo   %INSTALL_DIR%\uninstall.bat
echo.
pause
