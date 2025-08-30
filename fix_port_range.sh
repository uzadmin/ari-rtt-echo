#!/bin/bash

# Script to fix port range issues and restart services

echo "=== Fixing Port Range Issues ==="

# Stop any running containers
echo "Stopping existing containers..."
docker-compose down

# Rebuild the container to ensure new environment variables are used
echo "Rebuilding container with updated port range..."
docker-compose build

# Start the services
echo "Starting services with expanded port range..."
docker-compose up -d

# Wait a moment for services to start
sleep 5

# Check if services are running
echo "Checking service status..."
docker-compose ps

echo ""
echo "Services restarted with expanded port range (21000-31000)"
echo "This provides 10,001 ports which is sufficient for load testing"
echo ""
echo "To run the production test:"
echo "  ./run.sh prod"
echo ""
echo "To check logs:"
echo "  docker-compose logs -f asterisk"