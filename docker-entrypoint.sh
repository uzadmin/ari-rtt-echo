#!/bin/bash
set -e

# If we're running the default command (no arguments), start the full service
if [ $# -eq 0 ]; then
    # Start Asterisk
    echo "Starting Asterisk..."
    service asterisk start

    # Wait for Asterisk to be ready
    sleep 5

    # Check if Asterisk is running
    if ! asterisk -rx "core show version" > /dev/null 2>&1; then
        echo "Failed to start Asterisk"
        # Check Asterisk logs for more details
        if [ -f "/var/log/asterisk/messages" ]; then
            echo "Asterisk logs:"
            tail -20 /var/log/asterisk/messages
        fi
        exit 1
    fi

    echo "Asterisk started successfully"

    # Build the application if not already built
    if [ ! -f "bin/ari-service" ] || [ ! -f "bin/echo-server" ] || [ ! -f "bin/load-test" ] || [ ! -f "bin/load-test-new" ] || [ ! -f "bin/ari-server" ]; then
        echo "Building application..."
        go build -o bin/ari-service ./cmd/ari-service
        go build -o bin/echo-server ./cmd/echo
        go build -o bin/load-test ./cmd/load_test
        go build -o bin/load-test-new ./cmd/load_test_new
        go build -o bin/ari-server ./cmd/ari-server
    fi

    # Start the echo server in the background
    echo "Starting echo server..."
    ./bin/echo-server > logs/echo-server.log 2>&1 &
    echo "Echo server started"

    # Start the ARI server in the background
    echo "Starting ARI server..."
    ./bin/ari-server > logs/ari-server.log 2>&1 &
    echo "ARI server started"

    echo "Starting ARI service..."
    # Run the ARI service in a loop to prevent container shutdown
    while true; do
        ./bin/ari-service > logs/ari-service.log 2>&1
        echo "ARI service exited with code $?. Restarting in 5 seconds..."
        sleep 5
    done
else
    # If arguments are provided, execute them directly
    # But first start Asterisk if needed
    if [[ "$*" == *"asterisk"* ]] || [[ "$*" == *"ari"* ]]; then
        echo "Starting Asterisk..."
        service asterisk start
        sleep 3
    fi
    
    # Execute the provided command
    exec "$@"
fi