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
# Parameters are read from /etc/google/multinic-lustre.env, else defaults.
set -u

MD="http://metadata.google.internal/computeMetadata/v1/instance"
# Retries cover transient failures (connection refused, timeouts, 5xx) during
# early boot. A 404 is not retried, so walking network-interfaces/<idx> still
# stops at the first missing index.
md_get() {
	curl -s -f --retry 5 --retry-connrefused --retry-delay 2 \
		--connect-timeout 2 --max-time 5 -H "Metadata-Flavor: Google" \
		"${MD}/$1" 2>/dev/null || true
}

# shellcheck source=/dev/null
[[ -r /etc/google/multinic-lustre.env ]] && . /etc/google/multinic-lustre.env
LNET_OPTIONS="${MULTINIC_LNET_OPTIONS:-lnet_numa_range=1000000}"
TABLE_ID="${MULTINIC_TABLE_BASE:-101}"
RPF="${MULTINIC_RP_FILTER:-2}"

# Ensure /etc/iproute2/rt_tables exists so setup_rail() can register the
# lustre_table_<nic> name. Newer iproute2 (>= 6.5) does not create it.
RT_TABLES=/etc/iproute2/rt_tables
mkdir -p /etc/iproute2
touch "$RT_TABLES"

# Configure one secondary rail: $1 = Linux ifname, $2 = gateway from metadata.
# Returns non-zero if the rail cannot be used.
setup_rail() {
	local nic="$1" gw="$2" table="lustre_table_$1" ip

	# rp_filter=2 is required: replies from the Lustre servers arrive
	# on whichever rail LNet chose, not necessarily the rail owning the route,
	# and strict reverse-path filtering would drop them.
	sysctl -w "net.ipv4.conf.${nic}.rp_filter=${RPF}" || true

	if ! grep -q " ${table}\$" "$RT_TABLES"; then
		# Skip IDs another tool already named or populated; 253+ are reserved.
		while grep -q "^${TABLE_ID}[[:space:]]" "$RT_TABLES" ||
			[[ -n "$(ip route show table "$TABLE_ID" 2>/dev/null)" ]]; do
			TABLE_ID=$((TABLE_ID + 1))
			((TABLE_ID > 252)) && return 1
		done
		echo "${TABLE_ID} ${table}" >>"$RT_TABLES"
		TABLE_ID=$((TABLE_ID + 1))
	fi

	# Secondary NICs may take seconds after network-online.target for DHCP.
	# Bounded wait; never block boot indefinitely.
	for _ in $(seq 1 60); do
		ip="$(ip -4 -o addr show dev "$nic" | awk '{print $4}' | cut -d/ -f1 | head -n 1)"
		[[ -n "$ip" ]] && break
		sleep 1
	done
	[[ -n "$ip" && -n "$gw" ]] || return 1

	# Safe to re-run: the route is replaced and the rule is added only if
	# missing. Nothing is deleted, so a restart does not interrupt Lustre traffic.
	ip route replace default via "$gw" dev "$nic" table "$table" || return 1
	ip rule show | grep -qE "from ${ip//./\\.} lookup ${table}( |\$)" ||
		ip rule add from "$ip" table "$table"
}

PRIMARY_VPC="$(md_get network-interfaces/0/network)"
if [[ -z "$PRIMARY_VPC" ]]; then
	echo "multi-nic-lustre: no nic0 metadata"
	exit 0
fi

# Any non-primary interface in the same VPC as nic0 is a Lustre rail; GPU RDMA
# NICs live in their own VPC and are skipped. The Linux name is found by MAC.
# Only rails that were fully configured are handed to LNet.
RAILS=""
IDX=1
while NIC_VPC="$(md_get "network-interfaces/${IDX}/network")" && [[ -n "$NIC_VPC" ]]; do
	if [[ "$NIC_VPC" == "$PRIMARY_VPC" ]]; then
		MAC="$(md_get "network-interfaces/${IDX}/mac")"
		ADDR="$(grep -lxF "${MAC:-none}" /sys/class/net/*/address 2>/dev/null | head -n 1)"
		NIC="${ADDR%/address}"
		NIC="${NIC##*/}"
		if [[ -n "$NIC" ]] && setup_rail "$NIC" "$(md_get "network-interfaces/${IDX}/gateway")"; then
			RAILS="${RAILS},${NIC}"
		else
			echo "multi-nic-lustre: nic${IDX} (${NIC:-no ifname}) not usable, skipping"
		fi
	fi
	IDX=$((IDX + 1))
done

LUSTRE_CONF=/etc/modprobe.d/lustre.conf
MARKER="# Managed by multi-nic-lustre-setup.sh"
if [[ -z "$RAILS" ]]; then
	# Drop rails written on an earlier boot so LNet does not try NICs that are
	# no longer usable. The file is removed only if this script created it.
	grep -qxF "$MARKER" "$LUSTRE_CONF" 2>/dev/null && rm -f "$LUSTRE_CONF"
	echo "multi-nic-lustre: no secondary Lustre NIC configured, single-rail mode"
	exit 0
fi

PRIMARY_NIC="$(ip route show default | awk '/default/ {print $5}' | head -n 1)"
NETWORKS="tcp0(${PRIMARY_NIC:-eth0}${RAILS})"
mkdir -p /etc/modprobe.d
printf '%s\noptions lnet networks="%s" %s\n' "$MARKER" "$NETWORKS" "$LNET_OPTIONS" >"$LUSTRE_CONF"

# LNet reads these options only when the module loads.
LIVE="$(cat /sys/module/lnet/parameters/networks 2>/dev/null || true)"
if [[ -n "$LIVE" && "$LIVE" != "$NETWORKS" ]]; then
	echo "multi-nic-lustre: WARNING lnet already loaded with networks=${LIVE}; ${NETWORKS} takes effect after reboot"
fi

echo "multi-nic-lustre: configured ${NETWORKS}"
exit 0
