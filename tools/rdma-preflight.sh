#!/bin/bash
# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# A script intended to run as a pre-check to ensure full RDMA functionality

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

CQP_FAILURE="hardware initialization FAILED"
MAD_AGENT_ERROR="MAD agent registration failed"
MAD_QP_CREATE_FAILURE="create ib_mad QP1"
QP_ASYNC_EVENTS="qp async event"

get_rdma_interface() {
	# The RDMA device name under /sys/class/infiniband might be different
	# depending on the image.
	raw_list=$(ls /sys/class/infiniband/*/device/net)
	if ! ls /sys/class/infiniband/*/device/net >/dev/null 2>&1 || [[ -z "${raw_list}" ]]; then
		echo -e "${RED}No RDMA interfaces found.${NC}" >&2
		exit 1
	fi
	# If we have multiple, pick the first one
	local rdma_iface
	rdma_iface=$(echo "${raw_list}" | awk '{print $1}')
	if ! ethtool -i "${rdma_iface}" | grep -q "driver.*idpf"; then
		echo -e "${RED}RDMA interface ${rdma_iface} does not load the IDPF driver.${NC}" >&2
		exit 1
	fi
	echo "${rdma_iface}"
}

rdma_checks() {
	# Check if RDMA interface exists (if not found could indicate VF reset issues)
	local rdma_iface
	rdma_iface=$1
	if ! ifconfig | grep -q "${rdma_iface}"; then
		echo -e "${RED}No RDMA interface found.${NC}"
		return 1
	fi

	# Check for CQP failure
	if dmesg | grep -q "${CQP_FAILURE}"; then
		echo -e "${RED}CQP hardware initialization failed.${NC}"
		return 1
	fi

	# Check for MAD QP failure
	if dmesg | grep -q "${MAD_QP_CREATE_FAILURE}"; then
		echo -e "${RED}MAD QP registration failed.${NC}"
		return 1
	fi

	# Check for QP async events
	if dmesg | grep -q "${QP_ASYNC_EVENTS}"; then
		echo -e "${RED}Async event error detected.${NC}"
		return 1
	fi

	if dmesg | grep -q "${MAD_AGENT_ERROR}"; then
		echo -e "${RED}MAD agent error detected.${NC}"
		return 1
	fi

	return 0
}

# Run a rping loopback by starting server and client on the same VM
rping_loopback() {
	local rdma_iface
	rdma_iface=$1
	killall rping 2>/dev/null
	PRIMARY_IP=$(ip addr show "${rdma_iface}" | grep -Po "inet \K[\d.]+")

	# Run rping server on the primary IP
	rping -d -s -P -a "${PRIMARY_IP}" >/dev/null &

	# WARNING: This is a race condition - we don't really know if the server
	#          has started at this point... hopefully 10 seconds is enough.
	sleep 10

	echo "Listening on ${PRIMARY_IP}"

	rping -d -c -C 1 -a "${PRIMARY_IP}"
	rping_ret_code=$?
	if [[ ${rping_ret_code} -ne 0 ]]; then
		echo -e "${RED}rping loopback failed with return value of ${rping_ret_code}.${NC}"
		killall rping 2>/dev/null
		return 1
	fi
	killall rping 2>/dev/null
	return 0
}

vm_hostname=$(hostname -f)

rdma_iface=$(get_rdma_interface)
if ! rdma_iface=$(get_rdma_interface); then
	exit 1
fi

echo "Checking RDMA interface ${rdma_iface} on ${vm_hostname}"

if ! rdma_checks "${rdma_iface}"; then
	echo -e "${RED}Critical RDMA checks failed on ${vm_hostname}${NC}"
	exit 1
fi

if ! rping_loopback "${rdma_iface}"; then
	echo -e "${RED}Rping loopback failed on ${vm_hostname}${NC}"
	exit 1
fi

echo -e "${GREEN}Basic local checks passed on ${vm_hostname}${NC}"
