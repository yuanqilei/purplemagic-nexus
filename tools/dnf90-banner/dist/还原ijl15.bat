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

if exist "%GAME%\banner_cfg.txt.bak" (
    copy /y "%GAME%\banner_cfg.txt.bak" "%GAME%\banner_cfg.txt" >nul
    echo 已还原原 banner_cfg.txt
) else if exist "%GAME%\.dnf90_banner_cfg_created" (
    del /q "%GAME%\banner_cfg.txt" 2>nul
    del /q "%GAME%\.dnf90_banner_cfg_created" 2>nul
    echo 已移除补丁创建的 banner_cfg.txt
)

echo 请关闭游戏后重新启动。
echo.
pause
