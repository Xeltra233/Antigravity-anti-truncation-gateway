@echo off
chcp 65001 >nul 2>&1
title Antigravity Gateway

cd /d "%~dp0"

echo ===================================================
echo   Antigravity Gateway
echo ===================================================
echo.

"%~dp0gateway.exe"

if errorlevel 1 (
    echo.
    echo ===================================================
    echo [提示] 网关已退出。
    echo 故障排查：
    echo 1. 请检查同级目录下的 .env 文件，确认 UPSTREAM_BASE_URL 与 UPSTREAM_API_KEY 配置正确。
    echo 2. 检查端口（默认 8080）是否被其他程序占用。
    echo ===================================================
    pause
)
