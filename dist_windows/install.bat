@echo off
setlocal enabledelayedexpansion

echo =======================================================
echo   SingBox Client for Windows - Zero-UAC Installation
echo =======================================================
echo.

:: 1. Проверка прав администратора (нужна 1 раз только для регистрации службы)
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo [ERROR] Пожалуйста, запустите этот скрипт от имени Администратора (Run as Administrator)!
    echo Это требуется только 1 раз для регистрации системной службы Wintun.
    pause
    exit /b 1
)

set "PROGRAM_DATA_DIR=C:\ProgramData\singbox-tun"
set "RULE_SET_DIR=%PROGRAM_DATA_DIR%\rule-set"
set "APP_STARTUP_DIR=%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup"

echo [1/5] Создание каталога службы в ProgramData...
if not exist "%PROGRAM_DATA_DIR%" mkdir "%PROGRAM_DATA_DIR%"
if not exist "%RULE_SET_DIR%" mkdir "%RULE_SET_DIR%"

echo [2/5] Проверка и копирование исполняемых файлов и драйвера Wintun...
if not exist sing-box.exe (
    echo [ERROR] Файл sing-box.exe не найден в текущей папке установки!
    pause
    exit /b 1
)
if not exist wintun.dll (
    echo [ERROR] Файл wintun.dll не найден в текущей папке установки!
    pause
    exit /b 1
)
copy /Y sing-box.exe "%PROGRAM_DATA_DIR%\" >nul
copy /Y wintun.dll "%PROGRAM_DATA_DIR%\" >nul
if exist config.json copy /Y config.json "%PROGRAM_DATA_DIR%\" >nul
if exist geoip-ru.srs copy /Y geoip-ru.srs "%RULE_SET_DIR%\" >nul
if exist geosite-category-ru.srs copy /Y geosite-category-ru.srs "%RULE_SET_DIR%\" >nul
if exist "%~dp0..\bin\SingBoxTray.exe" copy /Y "%~dp0..\bin\SingBoxTray.exe" SingBoxTray.exe >nul

echo [3/5] Настройка разрешений ACL (Zero-UAC для обычных пользователей)...
:: Предоставляем группе Пользователи (Users / SID *S-1-5-32-545) право модификации в ProgramData\singbox-tun
icacls "%PROGRAM_DATA_DIR%" /grant *S-1-5-32-545:(OI)(CI)M /T /Q

echo [4/5] Установка и настройка службы Windows sing-box...
sc.exe stop sing-box >nul 2>&1
"%PROGRAM_DATA_DIR%\sing-box.exe" service uninstall >nul 2>&1

"%PROGRAM_DATA_DIR%\sing-box.exe" service install --work-dir "%PROGRAM_DATA_DIR%" -c "%PROGRAM_DATA_DIR%\config.json"
sc.exe config sing-box start= auto

:: Настройка восстановления службы при сбое бинарника
sc.exe failure sing-box reset= 60 actions= restart/5000/restart/10000/restart/30000

:: ПРЕДОСТАВЛЕНИЕ ПРАВ РЕСТАРТА СЛУЖБЫ ОБЫЧНЫМ ПОЛЬЗОВАТЕЛЯМ БЕЗ UAC:
:: Разрешаем группе Builtin\Users (BU) права SERVICE_START (RP) и SERVICE_STOP (WP)
sc.exe sdset sing-box "D:(A;;CCLCSWRPWPDTLOCRRC;;;SY)(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;BA)(A;;CCLCSWLOCRRC;;;IU)(A;;CCLCSWLOCRRC;;;SU)(A;;RPWP;;;BU)"

sc.exe start sing-box

echo [5/5] Настройка автозапуска трей-клиента SingBoxTray.exe...
if exist SingBoxTray.exe (
    copy /Y SingBoxTray.exe "%PROGRAM_DATA_DIR%\" >nul
    
    :: Создание ярлыка в автозагрузке пользователя
    powershell -Command "$ws = New-Object -ComObject WScript.Shell; $s = $ws.CreateShortcut('%APP_STARTUP_DIR%\SingBoxTray.lnk'); $s.TargetPath = '%PROGRAM_DATA_DIR%\SingBoxTray.exe'; $s.WorkingDirectory = '%PROGRAM_DATA_DIR%'; $s.Save()"
    
    :: Мгновенный запуск в сессии пользователя
    start "" "%PROGRAM_DATA_DIR%\SingBoxTray.exe"
)

echo.
echo =======================================================
echo   Установка успешно завершена!
echo   Служба Wintun запущена в фоне от SYSTEM.
echo   Трей работает в панели задач без запросов UAC.
echo =======================================================
pause
