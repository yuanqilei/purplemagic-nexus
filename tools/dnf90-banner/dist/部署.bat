@echo off
chcp 65001 >nul
setlocal
set "GAME=%~1"
if "%GAME%"=="" set "GAME=E:\D90"

echo ============================================
echo  部署 ijl15 代理到 %GAME%
echo ============================================

if not exist "%GAME%\DNF.exe" (
    echo [错误] 没找到 %GAME%\DNF.exe
    pause
    exit /b 1
)

if not exist "%~dp0ijl15.dll" (
    echo [错误] 当前目录缺少 ijl15.dll，请先运行 src\proxy\build.ps1 编译
    pause
    exit /b 1
)

rem 原版备份（只在第一次部署时做）
if not exist "%GAME%\ijl15.dll.bak" (
    copy /y "%GAME%\ijl15.dll" "%GAME%\ijl15.dll.bak" >nul
    echo [1/3] 已备份原版到 ijl15.dll.bak
) else (
    echo [1/3] 备份已存在，跳过
)

if not exist "%GAME%\ijl15_orig.dll" (
    copy /y "%GAME%\ijl15.dll" "%GAME%\ijl15_orig.dll" >nul
    echo [2/3] 已生成供代理加载的 ijl15_orig.dll
) else (
    echo [2/3] ijl15_orig.dll 已存在，跳过
)

copy /y "%~dp0ijl15.dll" "%GAME%\ijl15.dll" >nul
echo [3/3] 已替换 ijl15.dll

if exist "%GAME%\banner_cfg.txt" (
    if not exist "%GAME%\banner_cfg.txt.bak" (
        copy /y "%GAME%\banner_cfg.txt" "%GAME%\banner_cfg.txt.bak" >nul
        echo       已备份原 banner_cfg.txt
    )
) else (
    >"%GAME%\.dnf90_banner_cfg_created" echo created
)

if exist "%~dp0banner_cfg.txt" (
    copy /y "%~dp0banner_cfg.txt" "%GAME%\banner_cfg.txt" >nul
    echo       配置 banner_cfg.txt 已复制
)

echo.
echo 部署完成。启动游戏，进入赛利亚房间查看横幅。
echo 还原请运行：还原ijl15.bat
echo.
pause
