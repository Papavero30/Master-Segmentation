#!/bin/bash
set -e

echo "========================================"
echo "BrainNav GDCM Service Startup"
echo "========================================"

# Create necessary directories
echo "Creating directories..."
mkdir -p /app/logs
mkdir -p /app/temp
mkdir -p /app/input
mkdir -p /app/output

# Set permissions
chmod 755 /app/logs
chmod 755 /app/temp
chmod 755 /app/input
chmod 755 /app/output

# Check GDCM installation
echo "Checking GDCM installation..."
python3 -c "import gdcm; print(f'GDCM Version: {gdcm.Version.GetVersion()}')"

# Check dependencies
echo "Checking Python dependencies..."
python3 -c "import flask; print(f'Flask Version: {flask.__version__}')"
python3 -c "import requests; print(f'Requests Version: {requests.__version__}')"

# Environment variables
export FLASK_APP=gdcm_service.py
export FLASK_ENV=${FLASK_ENV:-production}
export FLASK_DEBUG=${FLASK_DEBUG:-false}

echo "Environment:"
echo "  FLASK_ENV: $FLASK_ENV"
echo "  FLASK_DEBUG: $FLASK_DEBUG"

# Backend integration settings
export BACKEND_URL=${BACKEND_URL:-http://brainnav-backend:8080}
export POSTGRES_URL=${POSTGRES_URL:-postgres://postgres:password@brainnav-db:5432/brainnav_db}

echo "Integration Settings:"
echo "  Backend URL: $BACKEND_URL"
echo "  Database URL: $POSTGRES_URL"

# Health check function
health_check() {
    echo "Performing health check..."
    
    # Check if service is responding
    timeout 30 bash -c 'until printf "" 2>>/dev/null >>/dev/tcp/$0/$1; do sleep 1; done' localhost 3000
    
    if [ $? -eq 0 ]; then
        echo "✅ GDCM Service is healthy and ready"
        return 0
    else
        echo "❌ GDCM Service health check failed"
        return 1
    fi
}

# Wait for dependencies if in Docker Compose
if [ "$WAIT_FOR_DEPS" = "true" ]; then
    echo "Waiting for dependencies..."

    # Wait for backend
    echo "Waiting for backend service..."
    backend_scheme=$(echo "$BACKEND_URL" | sed -E 's#^([a-zA-Z0-9+.-]+)://.*#\1#')
    backend_host=$(echo "$BACKEND_URL" | sed -E 's#^[a-zA-Z0-9+.-]+://([^/:]+).*#\1#')
    backend_port=$(echo "$BACKEND_URL" | sed -E 's#^[a-zA-Z0-9+.-]+://[^/:]+:([0-9]+).*#\1#')
    if [ -z "$backend_port" ]; then
        if [ "$backend_scheme" = "https" ]; then
            backend_port=443
        else
            backend_port=80
        fi
    fi
    end=$((SECONDS + 60))
    curl_opts=(--silent --max-time 5 --show-error --fail)
    if [ "$backend_scheme" = "https" ]; then
        curl_opts+=(--insecure)
    fi
    while [ $SECONDS -lt $end ]; do
        if printf "" 2>/dev/null >/dev/tcp/$backend_host/$backend_port; then
            if curl "${curl_opts[@]}" "$BACKEND_URL/health" >/dev/null 2>&1; then
                echo "✅ Backend is reachable"
                break
            fi
        fi
        sleep 2
    done
    if [ $SECONDS -ge $end ]; then
        echo "❌ Backend service not reachable within timeout"
        exit 1
    fi

    # Wait for database
    echo "Waiting for database service..."
    timeout 60 bash -c 'until printf "" 2>>/dev/null >>/dev/tcp/brainnav-db/5432; do sleep 2; done'

    echo "✅ Dependencies are ready"
fi

# Start the service
echo "Starting GDCM Service..."
echo "========================================"

# Run with Gunicorn for production or Flask dev server
if [ "$FLASK_ENV" = "production" ]; then
    echo "Starting with Gunicorn (Production Mode)..."
    exec gunicorn \
        --bind 0.0.0.0:3000 \
        --workers 4 \
        --worker-class sync \
        --worker-connections 1000 \
        --max-requests 1000 \
        --max-requests-jitter 100 \
        --timeout 300 \
        --keep-alive 60 \
        --access-logfile /app/logs/access.log \
        --error-logfile /app/logs/error.log \
        --log-level info \
        --capture-output \
        --enable-stdio-inheritance \
        gdcm_service:app
else
    echo "Starting with Flask Dev Server (Development Mode)..."
    exec python3 gdcm_service.py
fi
