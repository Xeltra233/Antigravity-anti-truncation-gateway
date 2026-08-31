@echo off
chcp 65001 >nul
title Antigravity Gateway

:: ===================================================
:: Antigravity Gateway 启动脚本
:: 请修改下方的环境变量，或保持默认配置
:: ===================================================

:: 上游 API 地址 (必填)
if "%UPSTREAM_BASE_URL%"=="" set UPSTREAM_BASE_URL=https://your-upstream-domain.com

:: 上游 API 密钥 (若上游不需要认证可留空并设置 UPSTREAM_AUTH_MODE=none)
if "%UPSTREAM_API_KEY%"=="" set UPSTREAM_API_KEY=sk-your-upstream-key

:: 本地网关监听端口 (默认 8080)
if "%PORT%"=="" set PORT=8080

:: 本地网关访问密钥 (下游客户端调用网关时使用的 API Key，留空则无需认证)
if "%API_KEY%"=="" set API_KEY=sk-antigravity-123456

echo ===================================================
echo   Antigravity Gateway 启动中...
echo   监听端口: %PORT%
echo   上游地址: %UPSTREAM_BASE_URL%
echo ===================================================

"%~dp0gateway.exe"

if %errorlevel% neq 0 (
    echo.
    echo [错误] 网关退出，错误码: %errorlevel%
    echo 请检查配置或端口占用情况。
    pause
)
