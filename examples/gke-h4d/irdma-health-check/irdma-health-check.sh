#!/bin/bash
# Copyright 2025 Google LLC
#
# RDMA Health Check for GKE Init Container on H4D Nodes
#
# Description:
# This script performs RDMA health checks before the main container starts.
# It's intended to run as an initContainer in a GKE Pod scheduled on H4D nodes.
#
# Functionality:
# 1. Checks if the RDMA link state is ACTIVE.
# 2. If the link is active, it performs a loopback bandwidth test using ib_send_bw,
#    binding to the IP of the 'eth1' interface.
# 3. If either test fails, it attempts a single recovery by bringing the 'eth1'
#    network interface down and then up.
# 4. It then re-runs the failed test.
# 5. If a test fails a second time, the script will exit with a non-zero status code (1),
#    causing the Pod to fail and preventing the main application from starting.

PATH=${PATH}:/usr/sbin:/usr/local/bin

# --- Configuration ---
# The RDMA device and network interface names as exposed in the Pod.
# Based on common GKE multi-network setups for RDMA.
RDMA_DEVICE="irdma0/1" # Corresponds to the device/port
# Corrected: The network interface for network "rdma-0" is eth1 inside the Pod.
NET_IFACE="eth1"    # Corresponds to eth1 in the Pod

# Number of loopback tests to run.
LOOPBACK_ITERATIONS=1

# Set to DRY_RUN to only print actions instead of taking them.
DRY_RUN=0

# --- Script Functions ---
# Log a message to stderr, visible in init container logs.
log() {
  echo "$(date): $1" >&2
}

# Check if the RDMA link is active. Returns 0 if active, 1 otherwise.
check_rdma_link() {
  log "Checking RDMA link state for $RDMA_DEVICE..."
  if rdma link show "$RDMA_DEVICE" | grep -q "state ACTIVE"; then
    log "RDMA link is ACTIVE."
    return 0
  else
    log "RDMA link is not ACTIVE."
    return 1
  fi
}

# Run the ib_send_bw loopback test, binding to the RDMA interface IP.
# Returns 0 if all tests pass, 1 otherwise.
run_loopback_test() {
  log "Running loopback test for $NET_IFACE ($RDMA_DEVICE)..."
  local success_count=0

  # Determine the IP address of the eth1 interface within the pod.
  RDMA_IP=$(ip -4 -o addr show dev "$NET_IFACE" | awk '{print $4}' | cut -d/ -f1)
  if [[ -z "$RDMA_IP" ]]; then
    log "ERROR: Could not determine IP address for interface $NET_IFACE. Skipping loopback test."
    return 1
  fi
  log "Discovered RDMA IP for $NET_IFACE: $RDMA_IP"

  for ((i=1; i<=LOOPBACK_ITERATIONS; i++)); do
    log "Running ib_send_bw iteration $i..."
    # Start the server in the background, binding to the RDMA IP
    # Using the full path: /usr/bin/ib_send_bw
    /usr/bin/ib_send_bw -F -n 5 -q 10 -s 8388608 --mr_per_qp --bind_source_ip="$RDMA_IP" &
    local server_pid=$!
    sleep 1 # Wait for the server to be ready

    # Run the client, connecting to the server on the RDMA IP
    # Using the full path: /usr/bin/ib_send_bw
    if /usr/bin/ib_send_bw -F -n 5 -q 10 -s 8388608 --mr_per_qp --bind_source_ip="$RDMA_IP" "$RDMA_IP"; then
      ((success_count++))
    else
      log "ib_send_bw client failed in iteration $i."
    fi
    # Clean up the server process
    kill $server_pid 2>/dev/null || true
    wait $server_pid 2>/dev/null
  done

  log "Loopback test result: $success_count/$LOOPBACK_ITERATIONS successful."
  if [ "$success_count" -eq "$LOOPBACK_ITERATIONS" ]; then
    log "Loopback test PASSED."
    return 0
  else
    log "Loopback test FAILED."
    return 1
  fi
}

# Attempt to recover the network interface by bouncing it.
try_recover_rdma() {
  log "Attempting to recover interface $NET_IFACE..."
  if [[ ${DRY_RUN} == 0 ]] ; then
    ifconfig "$NET_IFACE" down
    sleep 2
    ifconfig "$NET_IFACE" up
    sleep 5 # Allow time for the interface to initialize
  else
    log "DRY_RUN: Would have run ifconfig down/up on $NET_IFACE."
  fi
  log "Recovery attempt finished."
}


# --- Main Logic ---
log "Starting RDMA health check init container."

# 1. First, check the RDMA link state.
if ! check_rdma_link; then
  log "RDMA link check failed. Attempting recovery..."
  try_recover_rdma
  if ! check_rdma_link; then
    log "ERROR: RDMA link is not ACTIVE after recovery attempt. Failing pod."
    exit 1
  fi
  log "RDMA link check passed after recovery."
fi

# 2. If the link is good, perform the loopback test.
if ! run_loopback_test; then
  log "RDMA loopback test failed. Attempting recovery..."
  try_recover_rdma
  if ! run_loopback_test; then
    log "ERROR: RDMA loopback test failed after recovery attempt. Failing pod."
    exit 1
  fi
  log "RDMA loopback test passed after recovery."
fi

log "RDMA health checks passed. Init container exiting successfully."
exit 0
