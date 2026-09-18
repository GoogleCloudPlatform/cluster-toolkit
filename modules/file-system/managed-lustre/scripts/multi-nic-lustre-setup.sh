#!/bin/bash
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Configure Multi-NIC Managed Lustre rails: policy-based routing on each
# secondary rail + LNet module options.
#
# Exits 0 on every non-fatal condition. A node that cannot be configured must
# still boot and mount Lustre in single-rail mode.
#
# Parameters are read from /etc/google/multinic-lustre.env if present, else
# from instance metadata, else defaults. Keeping them out of this file is what
# lets the identical script be baked into an image without a respin per tune.
set -u

MD="http://metadata.google.internal/computeMetadata/v1/instance"
HDR="Metadata-Flavor: Google"
md_get() { curl -s -f -H "$HDR" "$1" 2>/dev/null || true; }

# shellcheck source=/dev/null
[[ -r /etc/google/multinic-lustre.env ]] && . /etc/google/multinic-lustre.env

LNET_OPTIONS="${MULTINIC_LNET_OPTIONS:-$(md_get "${MD}/attributes/multinic-lustre-lnet-options")}"
LNET_OPTIONS="${LNET_OPTIONS:-lnet_numa_range=1000000 lnet_peer_discovery_disabled=0}"
TABLE_ID="${MULTINIC_TABLE_BASE:-101}"
RPF="${MULTINIC_RP_FILTER:-2}"

# $1 (optional): comma-separated explicit NIC list.
SEC_NICS="$(echo "${1:-}" | tr ',' ' ' | xargs)"

if [[ -z "$SEC_NICS" ]]; then
	PRIMARY_VPC="$(md_get "${MD}/network-interfaces/0/network")"
	if [[ -z "$PRIMARY_VPC" ]]; then
		echo "multi-nic-lustre: no nic0 metadata"
		exit 0
	fi

	# Any non-primary interface in the SAME VPC as nic0 is a Lustre rail.
	# GPU RDMA rails live in their own VPC and are skipped automatically.
	# Resolve each secondary NIC's Linux interface name by matching its GCE metadata
	# MAC address against /sys/class/net/*/address (excluding loopback).
	IDX=1
	while true; do
		NIC_VPC="$(md_get "${MD}/network-interfaces/${IDX}/network")"
		[[ -z "$NIC_VPC" ]] && break
		if [[ "$NIC_VPC" == "$PRIMARY_VPC" ]]; then
			NIC_MAC="$(md_get "${MD}/network-interfaces/${IDX}/mac")"
			if [[ -n "$NIC_MAC" ]]; then
				for CAND in /sys/class/net/*; do
					CAND_NAME="$(basename "$CAND")"
					[[ "$CAND_NAME" == "lo" ]] && continue
					if [[ "$(cat "$CAND/address" 2>/dev/null)" == "$NIC_MAC" ]]; then
						SEC_NICS="$SEC_NICS $CAND_NAME"
						break
					fi
				done
			fi
		fi
		IDX=$((IDX + 1))
	done
	SEC_NICS="$(echo "$SEC_NICS" | xargs)"
fi

if [[ -z "$SEC_NICS" ]]; then
	echo "multi-nic-lustre: no secondary Lustre NIC found, single-rail mode"
	exit 0
fi

for NIC in $SEC_NICS; do
	# rp_filter=2 (loose) is required: replies from the Lustre servers arrive
	# on whichever rail LNet chose, not necessarily the rail owning the route,
	# and strict reverse-path filtering would drop them.
	sysctl -w "net.ipv4.conf.${NIC}.rp_filter=${RPF}" || true

	TABLE_NAME="lustre_table_${NIC}"
	grep -q " ${TABLE_NAME}\$" /etc/iproute2/rt_tables ||
		echo "${TABLE_ID} ${TABLE_NAME}" >>/etc/iproute2/rt_tables

	# Secondary NICs may take seconds after network-online.target for DHCP.
	# Bounded wait; never block boot indefinitely.
	for _ in $(seq 1 60); do
		[[ -n "$(ip -4 -o addr show dev "$NIC" | awk '{print $4}')" ]] && break
		sleep 1
	done

	SEC_IP="$(ip -4 -o addr show dev "$NIC" | awk '{print $4}' | cut -d/ -f1)"
	if [[ -z "$SEC_IP" ]]; then
		echo "multi-nic-lustre: $NIC no address, skipping"
		continue
	fi

	GW="$(echo "$SEC_IP" | awk -F. '{print $1"."$2"."$3".1"}')"
	ip route add default via "$GW" dev "$NIC" table "$TABLE_NAME" || true
	ip rule add from "$SEC_IP" table "$TABLE_NAME" || true
	TABLE_ID=$((TABLE_ID + 1))
done

PRIMARY_NIC="$(ip route show default | awk '/default/ {print $5}' | head -n 1)"
SEC_LIST="$(echo "$SEC_NICS" | tr ' ' ',')"
mkdir -p /etc/modprobe.d
echo "options lnet networks=\"tcp0(${PRIMARY_NIC:-eth0},${SEC_LIST})\" ${LNET_OPTIONS}" \
	>/etc/modprobe.d/lustre.conf

echo "multi-nic-lustre: configured rails ${PRIMARY_NIC} ${SEC_NICS}"
exit 0
