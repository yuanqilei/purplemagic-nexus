# =============================================================================
#  编译 ijl15.dll 代理
#  依赖：Visual Studio 2022 BuildTools（C++ x86 工具链）
#  用法：在本目录执行  .\build.ps1
# =============================================================================
$ErrorActionPreference = 'Stop'

$vcvars = 'C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvarsall.bat'
if (-not (Test-Path $vcvars)) {
    Write-Host '找不到 vcvarsall.bat，请先安装 VS2022 BuildTools（C++ 桌面开发）' -ForegroundColor Red
    exit 1
}

$output = Join-Path $PSScriptRoot 'ijl15.dll'
if (Test-Path $output) { Remove-Item $output -Force }

$args = 'call "{0}" x86 >nul && cd /d "{1}" && ' +
        'cl /nologo /O2 /MT /LD /EHsc /utf-8 ijl15_proxy.cpp ' +
        '/link /DEF:ijl15_proxy.def /OUT:"{2}" /MACHINE:X86 psapi.lib'
$command = $args -f $vcvars, $PSScriptRoot, $output

cmd /c $command

if (-not (Test-Path $output)) {
    Write-Host '编译失败' -ForegroundColor Red
    exit 1
}

Write-Host ('编译成功：' + $output + '  (' + (Get-Item $output).Length + ' 字节)') -ForegroundColor Green
Write-Host '—— 导出检查 ——' -ForegroundColor Cyan
cmd /c ('call "{0}" x86 >nul && dumpbin /nologo /exports "{1}"' -f $vcvars, $output)

# 清理中间产物
foreach ($ext in 'obj', 'lib', 'exp') {
    $f = Join-Path $PSScriptRoot ('ijl15_proxy.' + $ext)
    if (Test-Path $f) { Remove-Item $f -Force }
}
