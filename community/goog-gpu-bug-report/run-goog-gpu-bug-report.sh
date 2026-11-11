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

# Runs the GPU bug report container on a GCE VM.
#
# The container needs the host's NVIDIA driver userspace (nvidia-smi,
# libnvidia-ml, ...). If NVIDIA Container Toolkit 1.18.0 or newer is installed
# and its runtime mode is "auto" (the default), "jit-cdi" or "cdi", the
# container first runs through the toolkit's runtime, which injects them
# through CDI: in "auto" and "jit-cdi" mode it generates the CDI spec when the
# container is created (JIT-CDI), and in "cdi" mode it uses the host's CDI spec
# file. If the toolkit can't be used or that container cannot start (the runtime
# is not registered with Docker, the driver does not respond, ...), it runs with
# the host's driver directory bind-mounted (/var/lib/nvidia on COS, or /usr at
# /host_usr), which the image has on PATH and LD_LIBRARY_PATH. See README.md for
# setting up the toolkit.
#
# Usage:
#   bash run-goog-gpu-bug-report.sh [--image=IMAGE] [-- COLLECTOR_FLAGS...]
#
# Options:
#   --image=IMAGE  Collector image to run. Defaults to DEFAULT_IMAGE below.
#   --             Ends the launcher's options. The flags after it are passed to
#                  the collector in the container (app/goog-gpu-bug-report.py).
#
# Collector flags:
#   --gcs_bucket=BUCKET
#       Uploads the bug report archive to this Cloud Storage bucket. Without it,
#       the report is only written to /tmp/goog_gpu_bug_reports on the host.
#   --mft_kernel_modules=auto|required|never
#       Controls the NVIDIA MFT kernel modules on machine types that use MFT
#       (a4*). "auto" (default) loads them if possible and otherwise runs MFT
#       without them (driverless). "required" fails the run if they cannot be
#       loaded. "never" unloads and blocks them, so MFT runs driverless.
#
# Example:
#   bash run-goog-gpu-bug-report.sh -- --gcs_bucket=my-bucket

set -euo pipefail

readonly DEFAULT_IMAGE="us-central1-docker.pkg.dev/gce-ai-infra/goog-gpu-bug-report-repo/goog-gpu-bug-report:latest"
readonly CONTAINER_NAME="goog-gpu-bug-report"
# NVIDIA Container Toolkit 1.18.0 made JIT-CDI the runtime's default ("auto")
# mode. Older releases default to legacy mode, which leaves out nvidia-imex and
# nvidia-imex-ctl.
readonly MIN_TOOLKIT_VERSION="1.18.0"
# The collector exits with this code if the toolkit's container lacks nvidia-smi
# or nvidia-bug-report.sh (EXIT_CDI_INCOMPLETE in app/constants.py).
readonly EXIT_CDI_INCOMPLETE=3
# Standard `docker run` exit codes when the container cannot be created or
# started (https://docs.docker.com/engine/containers/run/#exit-status).
readonly EXIT_DOCKER_DAEMON_ERROR=125
readonly EXIT_DOCKER_CANNOT_INVOKE=126
readonly EXIT_DOCKER_CMD_NOT_FOUND=127

# Prints the NVIDIA Container Toolkit version, or nothing if it isn't installed.
# Sample `nvidia-ctk --version` output (parsed result: "1.18.0"):
#   NVIDIA Container Toolkit CLI version 1.18.0
#   commit: f8daa5e26de9fd7eb79259040b6dd5a52060048c
toolkit_version() {
	local output
	output="$(timeout 30 nvidia-ctk --version 2>/dev/null)" || return 0
	local -r pattern='NVIDIA Container Toolkit CLI version ([0-9.]+)'
	if [[ "${output}" =~ ${pattern} ]]; then
		echo "${BASH_REMATCH[1]}"
	fi
}

# Prints the NVIDIA Container Runtime mode set in the toolkit's config, or
# "auto" (the toolkit's default) if it isn't set.
# Sample `nvidia-ctk config` excerpt (parsed result: "auto"):
#   [nvidia-container-runtime]
#   #debug = "/var/log/nvidia-container-runtime.log"
#   log-level = "info"
#   mode = "auto"
#   runtimes = ["runc", "crun"]
toolkit_mode() {
	local mode
	mode="$(timeout 30 nvidia-ctk config 2>/dev/null | awk '
    /^\[/ { in_runtime = ($0 == "[nvidia-container-runtime]"); next }
    in_runtime && /^[[:space:]]*mode[[:space:]]*=/ {
      gsub(/^[^=]*=[[:space:]]*"|".*$/, ""); print; exit
    }')" || true
	echo "${mode:-auto}"
}

# version_at_least VERSION MINIMUM: true if VERSION >= MINIMUM.
version_at_least() {
	[[ -n "$1" ]] && printf '%s\n' "$2" "$1" | sort -C -V
}

run_collector() {
	docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
	docker run "$@"
}

main() {
	local image="${DEFAULT_IMAGE}"
	local -a collector_args=()
	while (($#)); do
		case "$1" in
		--image=*) image="${1#*=}" ;;
		--)
			shift
			collector_args=("$@")
			break
			;;
		*)
			echo "Unknown argument: $1" >&2
			exit 2
			;;
		esac
		shift
	done

	if ! command -v docker >/dev/null; then
		echo "docker is not installed." >&2
		exit 1
	fi

	local -a common_args=(
		--rm --name "${CONTAINER_NAME}"
		--privileged --network=host --uts=host --pid=host
		-e SYSTEMD_IGNORE_CHROOT=1
		-v /tmp:/tmp -v /dev:/dev -v /run:/run
	)
	# Host paths that are mounted at the same path if they exist. A ":ro" suffix
	# mounts the path read-only.
	local mount
	for mount in /var/log:ro /lib/modules /usr/src /etc/os-release:ro \
		/etc/lsb-release:ro /etc/machine-id:ro /etc/issue:ro /etc/nvidia-imex:ro; do
		if [[ -e "${mount%:ro}" ]]; then
			common_args+=(-v "${mount%:ro}:${mount}")
		fi
	done

	# Arguments for running the container with the NVIDIA Container Toolkit
	# runtime, which injects the host's NVIDIA driver files and GPU devices into
	# the container through CDI.
	local -a nct_args=(
		--runtime=nvidia
		-e NVIDIA_VISIBLE_DEVICES=all
		-e GOOG_GPU_BUG_REPORT_DRIVER_USERSPACE=nct_runtime_cdi
	)
	# The toolkit does not inject these, so mount the host's copies at the same
	# path. COS keeps them in /var/lib/nvidia/bin.
	local tool tool_path
	for tool in nvidia-bug-report.sh nvidia-networking-bug-report.py \
		nv-fabricmanager; do
		tool_path="$(
			PATH="${PATH}:/var/lib/nvidia/bin"
			command -v "${tool}" || true
		)"
		if [[ -n "${tool_path}" ]]; then
			nct_args+=(-v "${tool_path}:${tool_path}:ro")
		fi
	done

	local -a host_mount_args
	if [[ -d /var/lib/nvidia/bin ]]; then
		host_mount_args=(-v /var/lib/nvidia:/var/lib/nvidia:ro)
	else
		host_mount_args=(-v /usr:/host_usr:ro)
	fi

	if ! docker pull "${image}"; then
		echo "Could not pull ${image}; using the local copy if there is one." >&2
	fi

	local version mode fallback_reason="" rc
	version="$(toolkit_version)"
	if [[ -z "${version}" ]]; then
		fallback_reason="NVIDIA Container Toolkit not installed"
	elif ! version_at_least "${version}" "${MIN_TOOLKIT_VERSION}"; then
		fallback_reason="NVIDIA Container Toolkit ${version} is older than"
		fallback_reason+=" ${MIN_TOOLKIT_VERSION}"
	else
		mode="$(toolkit_mode)"
		case "${mode}" in
		auto | jit-cdi | cdi) ;;
		*) fallback_reason="NVIDIA Container Toolkit runtime mode is ${mode}" ;;
		esac
	fi

	if [[ -z "${fallback_reason}" ]]; then
		echo "Running with the NVIDIA Container Toolkit ${version} runtime" \
			"(mode ${mode})." >&2
		rc=0
		run_collector "${common_args[@]}" "${nct_args[@]}" "${image}" \
			"${collector_args[@]}" || rc=$?
		case "${rc}" in
		# Retry with host mounts only if docker run could not create or start
		# the container, or if the collector exited because CDI was incomplete.
		"${EXIT_DOCKER_DAEMON_ERROR}" | "${EXIT_DOCKER_CANNOT_INVOKE}" | \
			"${EXIT_DOCKER_CMD_NOT_FOUND}" | "${EXIT_CDI_INCOMPLETE}")
			fallback_reason="NVIDIA Container Toolkit run (mode ${mode}) failed"
			fallback_reason+=" with exit code ${rc}"
			;;
		*) exit "${rc}" ;;
		esac
	fi

	echo "${fallback_reason}; running with host mounts." >&2
	run_collector "${common_args[@]}" "${host_mount_args[@]}" \
		-e "GOOG_GPU_BUG_REPORT_DRIVER_USERSPACE=host_mounts (${fallback_reason})" \
		"${image}" "${collector_args[@]}"
}

main "$@"
