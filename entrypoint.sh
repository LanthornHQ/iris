#!/bin/bash
set -e

Xvfb :99 -screen 0 1920x1080x24 &
sleep 1

export DISPLAY=:99
export XAUTHORITY=/root/.Xauthority

AUTH_KEY=$(od -An -N16 -tx1 /dev/urandom | tr -d ' ')
xauth add :99 . "$AUTH_KEY" 2>/dev/null || true

export IRIS_HEADLESS=false
export IRIS_ANNOTATE_CLICKS=1

exec /usr/local/bin/iris "$@"
