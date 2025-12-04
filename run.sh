#!/bin/zsh

# Navigate to script directory
cd "$(dirname "$0")"

# Load environment variables from .env file
if [ -f .env ]; then
    echo "Loading environment variables from .env..."
    export $(grep -v '^#' .env | xargs)
fi

# Build the application
echo "Building video-service..."
go build -o bin/video-service ./cmd/server
if [ $? -ne 0 ]; then
    echo "Build failed!"
    exit 1
fi

# Kill existing process
echo "Stopping existing service..."
pkill -f video-service
sleep 1

# Start the service
echo "Starting video-service..."
echo "CDN_BASE_URL: ${CDN_BASE_URL:-not set}"
echo "GCS_BUCKET_NAME: ${GCS_BUCKET_NAME:-not set}"
./bin/video-service
