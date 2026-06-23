#!/bin/bash

BACKEND_DIR="$(cd "$(dirname "$0")/backend" && pwd)"
BINARY="$BACKEND_DIR/mujian"
PID_FILE="/tmp/mujian-backend.pid"

build() {
    echo "Building..."
    cd "$BACKEND_DIR"
    GOPROXY=direct go build -o mujian . 2>&1
    if [ $? -eq 0 ]; then
        echo "Build successful"
        return 0
    else
        echo "Build failed"
        return 1
    fi
}

start() {
    if [ -f "$PID_FILE" ]; then
        kill $(cat "$PID_FILE") 2>/dev/null
        rm "$PID_FILE"
    fi
    cd "$BACKEND_DIR"
    ./mujian > /tmp/mujian-backend.log 2>&1 &
    echo $! > "$PID_FILE"
    echo "Server started (PID: $!)"
}

restart() {
    if build; then
        start
    fi
}

# Initial build and start
if build; then
    start
    echo "Watching for changes..."
    
    # Watch for .go file changes
    fswatch -0 --event Updated "$BACKEND_DIR" | while read -d "" file; do
        if [[ "$file" == *.go ]]; then
            echo "Change detected: $file"
            restart
        fi
    done
else
    echo "Initial build failed"
    exit 1
fi
