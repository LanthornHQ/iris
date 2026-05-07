@echo off
SETLOCAL

set DOCKER_IMAGE=iris

if "%IRIS_API_KEY%"=="" set IRIS_API_KEY=
if "%IRIS_GROUNDING_URL%"=="" set IRIS_GROUNDING_URL=http://host.docker.internal:8000/v1/chat/completions

echo Building Docker image: %DOCKER_IMAGE%...
docker build -t %DOCKER_IMAGE% .
if %errorlevel% neq 0 (
    echo Docker build failed!
    exit /b %errorlevel%
)

echo.
echo Running Docker container %DOCKER_IMAGE% (interactive mode)...
docker run --rm ^
    -p 3000:3000 ^
    --add-host=host.docker.internal:host-gateway ^
    -e IRIS_TRANSPORT=http ^
    -e IRIS_ADDR=0.0.0.0:3000 ^
    -e IRIS_GROUNDING_URL="%IRIS_GROUNDING_URL%" ^
    %DOCKER_IMAGE%

ENDLOCAL