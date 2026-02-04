# ============================================================================
# END-TO-END INTEGRATION TEST SCRIPT
# Tests complete distributed segmentation workflow
# ============================================================================

import os
import sys
import time
import json
import requests
import redis
import numpy as np
from concurrent.futures import ThreadPoolExecutor, as_completed

# Configuration
BASE_URL = "https://localhost:8443/api"
REDIS_URL = "redis://localhost:6379/0"
REDIS_PASSWORD = "brainnav_secure_password_CHANGE_ME"

# Test credentials (update with your actual test user)
TEST_EMAIL = "test@brainnav.com"
TEST_PASSWORD = "test_password"

class Colors:
    HEADER = '\033[95m'
    OKBLUE = '\033[94m'
    OKCYAN = '\033[96m'
    OKGREEN = '\033[92m'
    WARNING = '\033[93m'
    FAIL = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'

def log_info(msg):
    print(f"{Colors.OKCYAN}[INFO]{Colors.ENDC} {msg}")

def log_success(msg):
    print(f"{Colors.OKGREEN}[SUCCESS]{Colors.ENDC} {msg}")

def log_error(msg):
    print(f"{Colors.FAIL}[ERROR]{Colors.ENDC} {msg}")

def log_warning(msg):
    print(f"{Colors.WARNING}[WARNING]{Colors.ENDC} {msg}")

def get_auth_token():
    """Authenticate and get JWT token"""
    log_info("Authenticating...")
    
    response = requests.post(
        f"{BASE_URL}/auth/login",
        json={"email": TEST_EMAIL, "password": TEST_PASSWORD},
        verify=False  # Skip SSL verification for self-signed cert
    )
    
    if response.status_code == 200:
        token = response.json()['access_token']
        log_success(f"Authenticated successfully")
        return token
    else:
        log_error(f"Authentication failed: {response.text}")
        sys.exit(1)

def submit_segmentation_task(token, scan_sid, slice_index):
    """Submit a segmentation task"""
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    response = requests.post(
        f"{BASE_URL}/segmentation/segment",
        headers=headers,
        json={"scan_sid": scan_sid, "slice_index": slice_index},
        verify=False
    )
    
    if response.status_code == 202:
        data = response.json()
        task_id = data['task_id']
        log_success(f"Task submitted: {task_id}")
        return task_id
    else:
        log_error(f"Task submission failed: {response.text}")
        return None

def get_task_status(token, task_id):
    """Get task status"""
    headers = {"Authorization": f"Bearer {token}"}
    
    response = requests.get(
        f"{BASE_URL}/segmentation/status/{task_id}",
        headers=headers,
        verify=False
    )
    
    if response.status_code == 200:
        return response.json()
    else:
        log_error(f"Failed to get task status: {response.text}")
        return None

def wait_for_completion(token, task_id, timeout=300):
    """Wait for task to complete"""
    log_info(f"Waiting for task {task_id} to complete...")
    
    start_time = time.time()
    last_progress = -1
    
    while time.time() - start_time < timeout:
        status = get_task_status(token, task_id)
        
        if not status:
            time.sleep(2)
            continue
        
        progress = status.get('progress', 0)
        task_status = status.get('status', 'unknown')
        
        # Print progress update
        if progress != last_progress:
            print(f"  Progress: {progress}% - Status: {task_status}")
            last_progress = progress
        
        if task_status == 'completed':
            log_success(f"Task {task_id} completed in {time.time() - start_time:.2f}s")
            return True
        elif task_status == 'failed':
            log_error(f"Task {task_id} failed")
            return False
        
        time.sleep(2)
    
    log_error(f"Task {task_id} timed out after {timeout}s")
    return False

def get_segmentation_result(token, task_id):
    """Download segmentation result"""
    headers = {"Authorization": f"Bearer {token}"}
    
    response = requests.get(
        f"{BASE_URL}/segmentation/result/{task_id}",
        headers=headers,
        verify=False
    )
    
    if response.status_code == 200:
        mask_data = response.content
        log_success(f"Downloaded result: {len(mask_data)} bytes")
        return mask_data
    else:
        log_error(f"Failed to download result: {response.text}")
        return None

def verify_redis_results(task_id, total_chunks):
    """Verify results are stored in Redis"""
    log_info(f"Verifying Redis storage for task {task_id}...")
    
    try:
        r = redis.Redis.from_url(REDIS_URL, password=REDIS_PASSWORD, decode_responses=False)
        
        # Check each chunk result
        for i in range(total_chunks):
            result_key = f"segmentation:result:{task_id}:{i}"
            
            if not r.exists(result_key):
                log_error(f"Chunk {i} result not found in Redis")
                return False
        
        log_success(f"All {total_chunks} chunks verified in Redis")
        return True
    except Exception as e:
        log_error(f"Redis verification failed: {e}")
        return False

def test_single_slice():
    """Test 1: Single slice segmentation"""
    print(f"\n{Colors.BOLD}=== TEST 1: Single Slice Segmentation ==={Colors.ENDC}")
    
    token = get_auth_token()
    
    # Submit task
    task_id = submit_segmentation_task(token, "test_scan_001", 50)
    if not task_id:
        return False
    
    # Wait for completion
    if not wait_for_completion(token, task_id):
        return False
    
    # Get result
    mask_data = get_segmentation_result(token, task_id)
    if not mask_data:
        return False
    
    # Verify Redis
    if not verify_redis_results(task_id, 4):  # Assuming 4 chunks per slice
        return False
    
    log_success("Test 1 PASSED")
    return True

def test_concurrent_tasks():
    """Test 2: Concurrent task processing with 3 workers"""
    print(f"\n{Colors.BOLD}=== TEST 2: Concurrent Task Processing ==={Colors.ENDC}")
    
    token = get_auth_token()
    
    # Submit 12 tasks simultaneously
    num_tasks = 12
    log_info(f"Submitting {num_tasks} concurrent tasks...")
    
    task_ids = []
    for i in range(num_tasks):
        task_id = submit_segmentation_task(token, "test_scan_002", i)
        if task_id:
            task_ids.append((i, task_id))
    
    log_info(f"Submitted {len(task_ids)} tasks")
    
    # Wait for all to complete
    start_time = time.time()
    completed = 0
    
    for slice_idx, task_id in task_ids:
        if wait_for_completion(token, task_id, timeout=600):
            completed += 1
    
    total_time = time.time() - start_time
    
    # Calculate metrics
    avg_time = total_time / num_tasks
    log_success(f"Completed {completed}/{num_tasks} tasks in {total_time:.2f}s")
    log_info(f"Average time per task: {avg_time:.2f}s")
    
    # Expected speedup with 3 workers vs 1 worker
    # Theoretical: 3x speedup (2.0s → 0.67s per task on average)
    # Actual: depends on network latency and queue overhead
    if completed == num_tasks:
        log_success("Test 2 PASSED")
        return True
    else:
        log_error(f"Test 2 FAILED: Only {completed}/{num_tasks} completed")
        return False

def test_load_performance():
    """Test 3: Performance and load testing"""
    print(f"\n{Colors.BOLD}=== TEST 3: Load Performance Test ==={Colors.ENDC}")
    
    token = get_auth_token()
    
    # Submit 20 tasks for stress testing
    num_tasks = 20
    log_info(f"Submitting {num_tasks} tasks for load test...")
    
    start_time = time.time()
    
    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [
            executor.submit(submit_segmentation_task, token, f"load_test_scan", i)
            for i in range(num_tasks)
        ]
        
        task_ids = [f.result() for f in as_completed(futures) if f.result()]
    
    submission_time = time.time() - start_time
    log_info(f"Submitted {len(task_ids)} tasks in {submission_time:.2f}s")
    
    # Wait for completion
    log_info("Waiting for all tasks to complete...")
    start_process_time = time.time()
    
    completed = 0
    for task_id in task_ids:
        if wait_for_completion(token, task_id, timeout=600):
            completed += 1
    
    processing_time = time.time() - start_process_time
    
    log_success(f"Completed {completed}/{len(task_ids)} tasks")
    log_info(f"Total processing time: {processing_time:.2f}s")
    log_info(f"Throughput: {completed / processing_time:.2f} tasks/second")
    
    if completed >= len(task_ids) * 0.9:  # 90% success rate
        log_success("Test 3 PASSED")
        return True
    else:
        log_error(f"Test 3 FAILED: Only {completed}/{len(task_ids)} completed")
        return False

def main():
    print(f"{Colors.BOLD}{Colors.HEADER}")
    print("=" * 70)
    print("  BrainNav Distributed Segmentation - Integration Tests")
    print("=" * 70)
    print(f"{Colors.ENDC}")
    
    # Disable SSL warnings for self-signed certificate
    import urllib3
    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    
    results = []
    
    try:
        # Run tests
        results.append(("Single Slice Segmentation", test_single_slice()))
        results.append(("Concurrent Task Processing", test_concurrent_tasks()))
        results.append(("Load Performance Test", test_load_performance()))
    except KeyboardInterrupt:
        log_warning("\nTests interrupted by user")
        sys.exit(1)
    except Exception as e:
        log_error(f"Unexpected error: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
    
    # Summary
    print(f"\n{Colors.BOLD}{Colors.HEADER}=== TEST SUMMARY ==={Colors.ENDC}")
    passed = sum(1 for _, result in results if result)
    total = len(results)
    
    for name, result in results:
        status = f"{Colors.OKGREEN}PASSED{Colors.ENDC}" if result else f"{Colors.FAIL}FAILED{Colors.ENDC}"
        print(f"  {name}: {status}")
    
    print(f"\n{Colors.BOLD}Total: {passed}/{total} tests passed{Colors.ENDC}")
    
    if passed == total:
        print(f"{Colors.OKGREEN}{Colors.BOLD}All tests passed! ✓{Colors.ENDC}")
        sys.exit(0)
    else:
        print(f"{Colors.FAIL}{Colors.BOLD}Some tests failed ✗{Colors.ENDC}")
        sys.exit(1)

if __name__ == "__main__":
    main()
