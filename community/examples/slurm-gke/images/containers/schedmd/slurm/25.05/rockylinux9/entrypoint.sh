#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (C) SchedMD LLC.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

# Additional arguments to pass to slurmd.
export SLURMD_OPTIONS="${SLURMD_OPTIONS:-} $*"

# The asserted CPU resource limit of the pod.
export POD_CPUS="${POD_CPUS:-0}"

# The asserted memory resource limit (in MB) of the pod.
export POD_MEMORY="${POD_MEMORY:-0}"

# calculateCoreSpecCount returns a value for CoreSpecCount for the pod.
#
# CoreSpecCount represents the number of cores that the slurmd/slurmstepd
# should not use. Effectively it is the difference of the host and the pod's
# resource limits. We have to convert CPUs to cores.
#
# Ref: https://slurm.schedmd.com/slurm.conf.html#OPT_CoreSpecCount
# Ref: https://slurm.schedmd.com/core_spec.html
function calculateCoreSpecCount() {
	local coreSpecCount=0
	local coreCount=0
	local threadCount=0

	coreCount="$(($(lscpu | gawk '/^Socket\(s\):/{ print $2 }') * $(lscpu | gawk '/^Core\(s\) per socket:/{ print $4 }')))"
	threadCount="$(lscpu | gawk '/^Thread\(s\) per core:/{ print $4 }')"
	coreSpecCount="$((coreCount - (POD_CPUS / threadCount)))"

	if ((coreSpecCount > 0)); then
		echo "$coreSpecCount"
	else
		echo "0"
	fi
}

# calculateMemSpecLimit returns a value for MemSpecLimit for the pod.
#
# MemSpecLimit represents the amount of memory that the slurmd/slurmstepd
# cannot use. Effectively it is the difference of the host and the pod's
# resource limits. We have to convert memory to MB.
#
# Ref: https://slurm.schedmd.com/slurm.conf.html#OPT_MemSpecLimit
function calculateMemSpecLimit() {
	local memSpecLimit=0
	local totalMemory=0

	totalMemory="$(gawk '/^MemTotal:/{ print $2 }' /proc/meminfo)"
	memSpecLimit="$(((totalMemory / 1024) - POD_MEMORY))"

	if ((memSpecLimit > 0)); then
		echo "$memSpecLimit"
	else
		echo "0"
	fi
}

# addConfItem shims the item into SLURMD_OPTIONS.
#
# This function will add `--conf` if it is not present in SLURMD_OPTIONS,
# otherwise will add the item into the argument of `--conf`.
function addConfItem() {
	local item="$1"
	local slurmdOptions=()
	local foundConf=0
	readarray -t slurmdOptions < <(echo -n "$SLURMD_OPTIONS" | gawk -v FPAT="([^ ]+)|[^ ]*((\"[^\"]+\")|('[^']+'))" '{ for (i=1; i<=NF; i++) print $i }')
	for i in "${!slurmdOptions[@]}"; do
		case "${slurmdOptions[$i]}" in
		--conf=*)
			foundConf=1
			local val="${slurmdOptions[$i]#--conf=}"
			val="$(echo -n "$val" | sed -e 's/[\\]*"//g' -e "s/[\\]*'//g")"
			slurmdOptions[$i]="--conf='${val} ${item}'"
			;;
		--conf)
			foundConf=1
			local j="$((i + 1))"
			local val="${slurmdOptions[$j]}"
			val="$(echo -n "$val" | sed -e 's/[\\]*"//g' -e "s/[\\]*'//g")"
			slurmdOptions[$j]="'${val} ${item}'"
			;;
		*) ;;
		esac
	done
	if ((foundConf == 0)); then
		slurmdOptions+=("--conf")
		slurmdOptions+=("'${item}'")
	fi
	export SLURMD_OPTIONS="${slurmdOptions[*]}"
}

function main() {
	mkdir -p /run/slurm/
	mkdir -p /var/spool/slurmd/

	local coreSpecCount=0
	if ((POD_CPUS > 0)); then
		coreSpecCount="$(calculateCoreSpecCount)"
	fi
	if ((coreSpecCount > 0)); then
		addConfItem "CoreSpecCount=${coreSpecCount}"
	fi

	local memSpecLimit=0
	if ((POD_MEMORY > 0)); then
		memSpecLimit="$(calculateMemSpecLimit)"
	fi
	if ((memSpecLimit > 0)); then
		addConfItem "MemSpecLimit=${memSpecLimit}"
	fi

	exec supervisord -c /etc/supervisor/supervisord.conf
}
main
