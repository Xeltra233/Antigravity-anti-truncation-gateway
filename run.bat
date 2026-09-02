@echo off
chcp 65001 >nul
title Antigravity Gateway

:: ===================================================
:: Antigravity Gateway 启动脚本
:: ===================================================

cd /d "%~dp0"

set "HAS_ENV=0"
if exist ".env" (
    set "HAS_ENV=1"
) else if exist "%~dp0.env" (
    set "HAS_ENV=1"
)

:: 如果未检测到 .env 且未注入系统环境变量，尝试自动从 .env.example 创建
if "%HAS_ENV%"=="0" (
    if "%UPSTREAM_BASE_URL%"=="" (
        if exist ".env.example" (
            echo ===================================================
            echo [提示] 未检测到 .env 配置文件。
            echo [提示] 正在自动从 .env.example 生成 .env 文件...
            copy ".env.example" ".env" >nul
            set "HAS_ENV=1"
            echo [提示] 已成功生成 .env！请编辑 .env 填入您的上游地址和密钥。
            echo ===================================================
            echo.
        )
    )
)

echo ===================================================
echo   Antigravity Gateway 启动中...
if "%HAS_ENV%"=="1" (
    echo   配置方式: 本地 .env 配置文件优先
)
if not "%UPSTREAM_BASE_URL%"=="" (
    echo   系统环境变量上游地址: %UPSTREAM_BASE_URL%
)
if not "%PORT%"=="" (
    echo   系统环境变量监听端口: %PORT%
)
echo ===================================================
echo.

"%~dp0gateway.exe"

if %errorlevel% neq 0 (
    echo.
    echo ===================================================
    echo [错误] 网关退出，错误码: %errorlevel%
    echo 排查建议：
    echo 1. 请检查同级目录下的 .env 文件中 UPSTREAM_BASE_URL 是否正确（如 https://api.openai.com）
    echo 2. 若上游服务需要鉴权，请检查 .env 中的 UPSTREAM_API_KEY 是否已填写
    echo 3. 检查端口（默认 8080）是否被其他程序占用
    echo ===================================================
    pause
)
