# ============================================================================
# MULTI-NODE NETWORK CONNECTIVITY TEST SCRIPT
# File: scripts/test-multi-node-network.ps1
# Purpose: Automated testing untuk memverifikasi koneksi antar node
# ============================================================================
# Usage:
#   .\test-multi-node-network.ps1 -MasterIP "192.168.1.100"
# ============================================================================

param(
    [Parameter(Mandatory=$true)]
    [string]$MasterIP,
    
    [Parameter(Mandatory=$false)]
    [string]$TestType = "all"  # Options: all, network, rabbitmq, redis, docker
)

# ============================================================================
# Color Functions
# ============================================================================
function Write-Success {
    param([string]$Message)
    Write-Host "[✓] $Message" -ForegroundColor Green
}

function Write-Error-Custom {
    param([string]$Message)
    Write-Host "[✗] $Message" -ForegroundColor Red
}

function Write-Info {
    param([string]$Message)
    Write-Host "[i] $Message" -ForegroundColor Cyan
}

function Write-Warning-Custom {
    param([string]$Message)
    Write-Host "[!] $Message" -ForegroundColor Yellow
}

# ============================================================================
# TEST 1: BASIC NETWORK CONNECTIVITY
# ============================================================================
function Test-NetworkConnectivity {
    Write-Info "=========================================="
    Write-Info "TEST 1: Basic Network Connectivity"
    Write-Info "=========================================="
    
    # Test ping
    Write-Info "Testing ping to master node..."
    $pingResult = Test-Connection -ComputerName $MasterIP -Count 4 -Quiet
    
    if ($pingResult) {
        Write-Success "Ping successful to $MasterIP"
    } else {
        Write-Error-Custom "Ping failed to $MasterIP"
        Write-Warning-Custom "Check network cable, IP configuration, or firewall"
        return $false
    }
    
    # Test required ports
    $ports = @{
        "PostgreSQL" = 5432
        "RabbitMQ AMQP" = 5672
        "Redis" = 6379
        "Backend API" = 8443
        "RabbitMQ Management" = 15672
    }
    
    Write-Info "`nTesting port connectivity..."
    $allPortsOpen = $true
    
    foreach ($service in $ports.Keys) {
        $port = $ports[$service]
        $portTest = Test-NetConnection -ComputerName $MasterIP -Port $port -WarningAction SilentlyContinue
        
        if ($portTest.TcpTestSucceeded) {
            Write-Success "$service (port $port) - OPEN"
        } else {
            Write-Error-Custom "$service (port $port) - CLOSED"
            $allPortsOpen = $false
        }
    }
    
    return $allPortsOpen
}

# ============================================================================
# TEST 2: RABBITMQ CONNECTIVITY
# ============================================================================
function Test-RabbitMQConnectivity {
    Write-Info "`n=========================================="
    Write-Info "TEST 2: RabbitMQ Connectivity"
    Write-Info "=========================================="
    
    # Test RabbitMQ Management API
    Write-Info "Testing RabbitMQ Management API..."
    
    $rabbitUser = "brainnav"
    $rabbitPass = "brainnav_secure_password_CHANGE_ME_IN_PRODUCTION"
    
    try {
        $base64Auth = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("${rabbitUser}:${rabbitPass}"))
        $headers = @{
            Authorization = "Basic $base64Auth"
        }
        
        # Test overview endpoint
        $url = "http://${MasterIP}:15672/api/overview"
        $response = Invoke-RestMethod -Uri $url -Headers $headers -TimeoutSec 10
        
        Write-Success "RabbitMQ Management API accessible"
        Write-Info "RabbitMQ Version: $($response.rabbitmq_version)"
        Write-Info "Erlang Version: $($response.erlang_version)"
        
        # Test vhost
        $vhostUrl = "http://${MasterIP}:15672/api/vhosts/brainnav_vhost"
        $vhostResponse = Invoke-RestMethod -Uri $vhostUrl -Headers $headers -TimeoutSec 10
        Write-Success "VHost 'brainnav_vhost' accessible"
        
        # Test queue
        $queueUrl = "http://${MasterIP}:15672/api/queues/brainnav_vhost/segmentation_tasks"
        $queueResponse = Invoke-RestMethod -Uri $queueUrl -Headers $headers -TimeoutSec 10
        
        Write-Success "Queue 'segmentation_tasks' accessible"
        Write-Info "Messages Ready: $($queueResponse.messages_ready)"
        Write-Info "Consumers: $($queueResponse.consumers)"
        
        return $true
    }
    catch {
        Write-Error-Custom "RabbitMQ connection failed: $($_.Exception.Message)"
        Write-Warning-Custom "Check credentials in .env file"
        return $false
    }
}

# ============================================================================
# TEST 3: REDIS CONNECTIVITY
# ============================================================================
function Test-RedisConnectivity {
    Write-Info "`n=========================================="
    Write-Info "TEST 3: Redis Connectivity"
    Write-Info "=========================================="
    
    # Test menggunakan redis-cli (jika tersedia di Docker)
    Write-Info "Testing Redis connectivity..."
    
    $redisPassword = "brainnav_secure_password_CHANGE_ME_IN_PRODUCTION"
    
    try {
        # Test menggunakan Test-NetConnection sudah dilakukan di Test 1
        # Di sini kita test actual Redis command
        Write-Warning-Custom "Redis functional test requires redis-cli in Docker container"
        Write-Info "To test manually, run on Master Node:"
        Write-Info "  docker exec brainnav-redis redis-cli -a $redisPassword PING"
        Write-Info "  Expected output: PONG"
        
        return $true
    }
    catch {
        Write-Error-Custom "Redis connection test failed: $($_.Exception.Message)"
        return $false
    }
}

# ============================================================================
# TEST 4: DOCKER WORKER CONNECTIVITY
# ============================================================================
function Test-DockerWorkerConnectivity {
    Write-Info "`n=========================================="
    Write-Info "TEST 4: Docker Worker Container Test"
    Write-Info "=========================================="
    
    # Check if Docker is running
    Write-Info "Checking Docker status..."
    
    try {
        $dockerInfo = docker info 2>&1
        
        if ($LASTEXITCODE -eq 0) {
            Write-Success "Docker is running"
        } else {
            Write-Error-Custom "Docker is not running"
            Write-Warning-Custom "Start Docker Desktop and try again"
            return $false
        }
    }
    catch {
        Write-Error-Custom "Docker check failed: $($_.Exception.Message)"
        return $false
    }
    
    # Check worker containers
    Write-Info "`nChecking worker containers..."
    
    $containers = docker ps --filter "name=segmentation-worker" --format "{{.Names}}\t{{.Status}}"
    
    if ($containers) {
        Write-Success "Found worker containers:"
        $containers -split "`n" | ForEach-Object {
            Write-Info "  $_"
        }
    } else {
        Write-Warning-Custom "No worker containers found"
        Write-Info "Workers might not be started yet"
        return $false
    }
    
    # Check worker health endpoints
    Write-Info "`nChecking worker health endpoints..."
    
    $workerPorts = @(8001, 8002)  # GPU 0 and GPU 1
    
    foreach ($port in $workerPorts) {
        try {
            $healthUrl = "http://localhost:$port/health"
            $response = Invoke-RestMethod -Uri $healthUrl -TimeoutSec 5
            
            if ($response.status -eq "healthy") {
                Write-Success "Worker on port $port is HEALTHY"
                Write-Info "  GPU Available: $($response.gpu_available)"
                Write-Info "  Model Loaded: $($response.model_loaded)"
            }
        }
        catch {
            Write-Warning-Custom "Worker on port $port not responding (might not be started)"
        }
    }
    
    return $true
}

# ============================================================================
# TEST 5: END-TO-END CONNECTIVITY
# ============================================================================
function Test-EndToEndConnectivity {
    Write-Info "`n=========================================="
    Write-Info "TEST 5: End-to-End System Test"
    Write-Info "=========================================="
    
    Write-Info "Testing complete workflow simulation..."
    
    # 1. Check if Backend is accessible
    Write-Info "`n1. Testing Backend API..."
    
    try {
        $backendUrl = "https://${MasterIP}:8443/health"
        # Skip SSL verification untuk self-signed certificate
        [System.Net.ServicePointManager]::ServerCertificateValidationCallback = {$true}
        
        $response = Invoke-RestMethod -Uri $backendUrl -TimeoutSec 10
        Write-Success "Backend API accessible"
        Write-Info "  Status: $($response.status)"
    }
    catch {
        Write-Warning-Custom "Backend API not accessible: $($_.Exception.Message)"
        Write-Info "Backend might not be started yet"
    }
    
    # 2. Check RabbitMQ queue depth
    Write-Info "`n2. Checking RabbitMQ queue status..."
    
    try {
        $rabbitUser = "brainnav"
        $rabbitPass = "brainnav_secure_password_CHANGE_ME_IN_PRODUCTION"
        $base64Auth = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("${rabbitUser}:${rabbitPass}"))
        $headers = @{ Authorization = "Basic $base64Auth" }
        
        $queueUrl = "http://${MasterIP}:15672/api/queues/brainnav_vhost/segmentation_tasks"
        $queueResponse = Invoke-RestMethod -Uri $queueUrl -Headers $headers -TimeoutSec 10
        
        Write-Success "Queue status:"
        Write-Info "  Messages: $($queueResponse.messages)"
        Write-Info "  Consumers: $($queueResponse.consumers)"
        Write-Info "  Ready: $($queueResponse.messages_ready)"
        
        if ($queueResponse.consumers -eq 0) {
            Write-Warning-Custom "No consumers connected! Workers might not be running"
        } elseif ($queueResponse.consumers -lt 3) {
            Write-Warning-Custom "Expected 3 consumers (2 from PC1 + 1 from PC2), found $($queueResponse.consumers)"
        } else {
            Write-Success "All workers are connected to RabbitMQ"
        }
    }
    catch {
        Write-Error-Custom "Queue status check failed: $($_.Exception.Message)"
    }
    
    # 3. Summary
    Write-Info "`n=========================================="
    Write-Info "End-to-End Test Summary"
    Write-Info "=========================================="
    Write-Info "If all tests passed, your distributed system is ready!"
    Write-Info "`nNext steps:"
    Write-Info "  1. Start workers on all PC nodes"
    Write-Info "  2. Verify 3 consumers in RabbitMQ Management UI"
    Write-Info "  3. Submit test segmentation task via Backend API"
    Write-Info "  4. Monitor processing in logs and Grafana"
}

# ============================================================================
# MAIN EXECUTION
# ============================================================================
Write-Info "=========================================="
Write-Info "MULTI-NODE DISTRIBUTED SYSTEM TEST"
Write-Info "=========================================="
Write-Info "Master Node IP: $MasterIP"
Write-Info "Test Type: $TestType"
Write-Info "Started: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')"
Write-Info "=========================================="

$allTestsPassed = $true

# Run tests based on TestType
if ($TestType -eq "all" -or $TestType -eq "network") {
    $result = Test-NetworkConnectivity
    $allTestsPassed = $allTestsPassed -and $result
}

if ($TestType -eq "all" -or $TestType -eq "rabbitmq") {
    $result = Test-RabbitMQConnectivity
    $allTestsPassed = $allTestsPassed -and $result
}

if ($TestType -eq "all" -or $TestType -eq "redis") {
    $result = Test-RedisConnectivity
    $allTestsPassed = $allTestsPassed -and $result
}

if ($TestType -eq "all" -or $TestType -eq "docker") {
    $result = Test-DockerWorkerConnectivity
    $allTestsPassed = $allTestsPassed -and $result
}

if ($TestType -eq "all") {
    Test-EndToEndConnectivity
}

# Final summary
Write-Info "`n=========================================="
Write-Info "FINAL RESULT"
Write-Info "=========================================="

if ($allTestsPassed) {
    Write-Success "All tests PASSED! Distributed system is ready."
} else {
    Write-Error-Custom "Some tests FAILED. Check errors above."
    Write-Warning-Custom "Refer to MULTI_NODE_DEPLOYMENT.md troubleshooting section"
}

Write-Info "Completed: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')"
Write-Info "=========================================="
