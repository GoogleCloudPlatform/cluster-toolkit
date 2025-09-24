#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (C) SchedMD LLC.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

# Additional arguments to pass to slurmctld.
export SLURMCTLD_OPTIONS="${SLURMCTLD_OPTIONS:-} $*"

function main() {
	mkdir -p /run/slurmctld/

	exec supervisord -c /etc/supervisor/supervisord.conf
}
main
