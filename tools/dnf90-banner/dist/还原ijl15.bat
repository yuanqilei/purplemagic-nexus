@echo off
chcp 65001 >nul
setlocal
set "GAME=%~1"
if "%GAME%"=="" set "GAME=E:\D90"

echo ============================================
echo  还原 %GAME%\ijl15.dll
echo ============================================

if not exist "%GAME%\ijl15.dll.bak" (
    echo [错误] 找不到 %GAME%\ijl15.dll.bak，无法还原
    pause
    exit /b 1
)

copy /y "%GAME%\ijl15.dll.bak" "%GAME%\ijl15.dll" >nul
echo 已还原为原版 ijl15.dll
echo 请关闭游戏后重新启动。
echo.
pause
