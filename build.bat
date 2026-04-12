@echo off
echo Running tests...
go test ./...
if errorlevel 1 (
    echo.
    echo Tests FAILED. Build aborted.
    exit /b 1
)
echo.
echo Tests passed. Building binary...
go build -o IncottDriver.exe -ldflags="-H windowsgui" .
if errorlevel 1 (
    echo Build FAILED.
    exit /b 1
)
echo Build successful: IncottDriver.exe
