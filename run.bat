@echo off
REM Run the AI Gateway API on Windows.
REM Loads environment variables from .env (if present), then starts the server.

setlocal enabledelayedexpansion
cd /d "%~dp0"

REM --- Load .env (lines starting with # are ignored) ---
if exist ".env" (
    echo Loading .env ...
    for /f "usebackq eol=# tokens=1,* delims==" %%a in (".env") do (
        set "_k=%%a"
        set "_v=%%b"
        if not "!_k!"=="" set "!_k!=!_v!"
    )
) else (
    echo No .env found ^(copy .env.example to .env to configure^). Using defaults.
)

REM --- Check Go is available ---
where go >nul 2>nul
if errorlevel 1 (
    echo ERROR: Go is not on PATH. Install it: winget install GoLang.Go
    exit /b 1
)

echo Starting gateway on port %PORT% ...
go run .

endlocal
