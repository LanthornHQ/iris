#!/bin/bash
set -e

CONTAINER_NAME="iris-integration-test"
IMAGE_NAME="lanthornhq/iris"
PORT=3000
MAX_ATTEMPTS=30
SLEEP_INTERVAL=1

echo "==> Preparing integration test environment..."

# 1. Clean up any existing container with the same name
if docker ps -a --format '{{.Names}}' | grep -Eq "^${CONTAINER_NAME}\$"; then
    echo "Found leftover container '${CONTAINER_NAME}'. Cleaning it up..."
    docker stop "${CONTAINER_NAME}" >/dev/null 2>&1 || true
    docker rm "${CONTAINER_NAME}" >/dev/null 2>&1 || true
fi

# 2. Setup cleanup trap
cleanup() {
    echo "==> Cleaning up container..."
    docker stop "${CONTAINER_NAME}" >/dev/null 2>&1 || true
    docker rm "${CONTAINER_NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

# 3. Start the container in background
echo "==> Starting container '${CONTAINER_NAME}' on port ${PORT}..."
docker run -d \
    --name "${CONTAINER_NAME}" \
    -p "${PORT}:3000" \
    -e IRIS_ADDR="0.0.0.0:3000" \
    "${IMAGE_NAME}"

# 4. Poll health endpoint
echo "==> Polling health endpoint at http://localhost:${PORT}/health..."
success=false
for i in $(seq 1 "${MAX_ATTEMPTS}"); do
    if response=$(curl -s -f "http://localhost:${PORT}/health" 2>/dev/null); then
        # Check if the response contains status "ok"
        if echo "${response}" | grep -q '"status":"ok"'; then
            echo "Health check passed! Response: ${response}"
            success=true
            break
        fi
    fi
    echo "Attempt $i/${MAX_ATTEMPTS}: Health check not ready yet, retrying..."
    sleep "${SLEEP_INTERVAL}"
done

if [ "${success}" = false ]; then
    echo "ERROR: Health check timed out after $((MAX_ATTEMPTS * SLEEP_INTERVAL)) seconds."
    echo "==> Container Logs:"
    docker logs "${CONTAINER_NAME}"
    exit 1
fi

# 5. Optional check for metrics endpoint
echo "==> Verifying metrics endpoint at http://localhost:${PORT}/metrics..."
if curl -s -f "http://localhost:${PORT}/metrics" >/dev/null; then
    echo "Metrics endpoint verified successfully!"
else
    echo "ERROR: Metrics endpoint check failed!"
    exit 1
fi

echo "==> Integration tests PASSED successfully!"
