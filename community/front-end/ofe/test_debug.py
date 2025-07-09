#!/usr/bin/env python3

# Test script to verify status mapping logic
def _map_slurm_status(slurm_status, exit_code=None, additional_states=None):
    """Map SLURM job status to OFE job status, considering exit code for completion and additional states for running jobs"""
    print(f"_map_slurm_status called with: slurm_status={slurm_status}, exit_code={exit_code}, additional_states={additional_states}")
    
    status_mapping = {
        'PENDING': 'q',
        'CONFIGURING': 'q',
        'RUNNING': 'r',
        'COMPLETING': 'r',
        'SUSPENDED': 'q',
        'REQUEUED': 'q',
        'FAILED': 'e',
        'CANCELLED': 'e',
        'TIMEOUT': 'e',
        'PREEMPTED': 'e',
    }

    # Handle COMPLETED status based on exit code
    if slurm_status == 'COMPLETED':
        if exit_code is not None:
            # For this test, assume successful if exit code is 0
            return 'c' if exit_code == 0 else 'e'
        else:
            # If no exit code available, default to completed successfully
            return 'c'

    # Special handling for RUNNING jobs with additional states
    if slurm_status == 'RUNNING' and additional_states:
        # Jobs that are "RUNNING" but still configuring/powering up should be treated as queued
        configuration_states = ['CONFIGURING', 'POWER_UP_NODE', 'BOOT_FAIL', 'NODE_FAIL', 'RESIZING']
        if any(state in configuration_states for state in additional_states):
            print(f"Job with RUNNING status has configuration states {additional_states}, mapping to queued")
            return 'q'
        else:
            print(f"Job with RUNNING status has non-configuration states {additional_states}, keeping as running")

    return status_mapping.get(slurm_status, 'n')

# Test cases based on the daemon logs
print("=== Test Cases ===")

# Case 1: RUNNING job with CONFIGURING and POWER_UP_NODE states (should be 'q')
print("\n1. RUNNING job with configuration states:")
result = _map_slurm_status('RUNNING', additional_states=['CONFIGURING', 'POWER_UP_NODE'])
print(f"Result: {result} (expected: q)")

# Case 2: Simulate what Django might be receiving (test with None additional_states)
print("\n2. RUNNING job with None additional states:")
result = _map_slurm_status('RUNNING', additional_states=None)
print(f"Result: {result} (expected: r)")

# Case 3: Simulate what the daemon is supposed to send
print("\n3. Simulating daemon data format:")
data = {
    'slurm_jobid': 1,
    'slurm_status': 'RUNNING',
    'slurm_additional_states': ['CONFIGURING', 'POWER_UP_NODE']
}
states_for_mapping = data.get('slurm_additional_states', [])
result = _map_slurm_status(data['slurm_status'], additional_states=states_for_mapping)
print(f"Result: {result} (expected: q)")
