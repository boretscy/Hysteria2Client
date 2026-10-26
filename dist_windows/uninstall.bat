@echo off
echo =======================================================
echo   Удаление SingBox Client
echo =======================================================
echo.

net session >nul 2>&1
if %errorLevel% neq 0 (
    echo [ERROR] Запустите скрипт от имени Администратора!
    pause
    exit /b 1
)

echo [1/3] Остановка трей-клиента...
taskkill /F /IM SingBoxTray.exe >nul 2>&1
del /F /Q "%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup\SingBoxTray.lnk" >nul 2>&1

echo [2/3] Удаление службы sing-box...
sc.exe stop sing-box >nul 2>&1
"C:\ProgramData\singbox-tun\sing-box.exe" service uninstall >nul 2>&1

echo [3/3] Очистка каталогов...
del /F /Q "C:\ProgramData\singbox-tun\wintun.dll" >nul 2>&1
del /F /Q "C:\ProgramData\singbox-tun\sing-box.exe" >nul 2>&1

echo.
echo Удаление завершено!
pause
