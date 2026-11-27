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

# Fail-open installer for Google Cloud ML Diagnostics (mldiagnostics-daemon /
# diagon-daemon.service). Omits `set -e` because bash disables errexit inside
# tested conditions; every error rolls back uncommitted APT changes, logs a
# WARNING, and exits 0 so VM boot continues uninterrupted.
set -uo pipefail

# --- Constants & Configuration ---
readonly SCRIPT_NAME="install_ml_diagnostics_agent"
readonly PACKAGE_NAME="mldiagnostics-daemon"
readonly SERVICE_NAME="diagon-daemon.service"
readonly APT_LOCK_TIMEOUT="30" APT_HTTP_TIMEOUT="15" APT_RETRIES="1" FULL_UPDATE_RETRY_DEADLINE_SECS="120"
readonly AR_TRANSPORT_PACKAGE="apt-transport-artifact-registry"
readonly AR_TRANSPORT_SUITE="apt-transport-artifact-registry-stable"

# Internal overrides for staging/testing; defaults target production repos.
APT_REPO_URL="${DIAGON_APT_REPO_URL:-ar+https://us-central1-apt.pkg.dev/projects/hypercomputediagon-release}"
APT_SUITE="${DIAGON_APT_SUITE:-daemon-apt}"
APT_COMPONENT="${DIAGON_APT_COMPONENT:-main}"
GPG_KEY_URL="${DIAGON_GPG_KEY_URL:-https://us-central1-apt.pkg.dev/doc/repo-signing-key.gpg}"
CHANNEL="${DIAGON_CHANNEL:-hypercomputecluster.googleapis.com/diagnostics}"

# System paths (unconditionally initialized so runtime invocations ignore env
# overrides, while sourced unit tests can redirect to temp directories).
APT_SOURCES_LIST="/etc/apt/sources.list" APT_SOURCES_DIR="/etc/apt/sources.list.d"
AR_TRANSPORT_KEYRING="/usr/share/keyrings/google-cloud-ar-transport.gpg"
AR_TRANSPORT_LIST="/etc/apt/sources.list.d/google-cloud-ar-transport.list"
DIAGON_KEYRING="/usr/share/keyrings/diagon-daemon.gpg"
DIAGON_LIST="/etc/apt/sources.list.d/diagon-daemon.list"
BINARY_PATH="/usr/bin/mldiagnostics_extension" SERVICE_FILE="/etc/systemd/system/diagon-daemon.service"

VERSION="" TMP_DIR="" PKG_VERSION=""
INSTALL_SUCCEEDED=0 RESTORE_HOLD_ON_EXIT=0 SHOW_HELP=0 SKIP_INSTALL=0 PKG_INSTALLED=0 PKG_HELD=0 PACKAGE_CHANGED=0
TRACKED_FILES=() MATCHED_APT_SOURCES=()

# --- Logging, Temp Files & Rollback ---
log_msg() {
	local level="$1" prio="$2" line
	shift 2
	line="[$(date -u +'%Y-%m-%dT%H:%M:%SZ')] ${level}: ${SCRIPT_NAME}: $*"
	if [[ "${level}" == "WARNING" ]]; then echo "${line}" >&2; else echo "${line}"; fi
	if command -v logger >/dev/null 2>&1; then logger -t "${SCRIPT_NAME}" -p "${prio}" -- "${level}: $*" || true; fi
}

log_info() { log_msg INFO user.info "$*"; }
log_warn() { log_msg WARNING user.warning "$*"; }
run_or_warn() {
	local msg="$1"
	shift
	"$@" && return 0
	log_warn "${msg}"
	return 1
}
file_mode() { stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1" 2>/dev/null || true; }

usage() {
	cat <<EOF
Usage: ${SCRIPT_NAME}.sh [--version=VERSION] [-h|--help]
Install and start Google Cloud ML Diagnostics (${PACKAGE_NAME} / ${SERVICE_NAME})
on Ubuntu x86_64 VMs (tested on Ubuntu 22.04 and 24.04). Fail-open: errors log a warning and exit 0.
  --version=VERSION  Exact Debian version to install and lock (apt-mark hold).
                     Empty installs the latest release on new VMs.
  -h, --help         Display this help message and exit.
EOF
}

init_tmp_dir() {
	[[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]] && return 0
	if ! TMP_DIR="$(mktemp -d)" || ! mkdir -m 0700 "${TMP_DIR}/gnupg" "${TMP_DIR}/backups"; then
		log_warn "Failed to initialize temporary working directory."
		return 1
	fi
}

install_atomic_file() {
	local src="$1" dest="$2" mode="$3" dest_dir staging_file
	dest_dir="$(dirname "${dest}")"
	[[ -d "${dest_dir}" ]] || run_or_warn "Failed to create directory ${dest_dir}." install -d -m 0755 "${dest_dir}" || return 1
	staging_file="$(mktemp "${dest_dir}/.$(basename "${dest}").XXXXXX")" || {
		log_warn "Failed to create staging file in ${dest_dir}."
		return 1
	}
	if ! cp "${src}" "${staging_file}" || ! chmod "${mode}" "${staging_file}" || ! mv -f "${staging_file}" "${dest}"; then
		rm -f -- "${staging_file}" || true
		log_warn "Failed to atomically install ${dest}."
		return 1
	fi
}

install_tracked_file() {
	local src="$1" dest="$2" mode="$3" cur_mode f tracked=0 idx
	if [[ -f "${dest}" ]] && cmp -s "${src}" "${dest}"; then
		cur_mode="$(file_mode "${dest}")"
		[[ "${cur_mode}" == "${mode#0}" || "${cur_mode}" == "${mode}" ]] && return 0
		run_or_warn "Failed to set permissions ${mode} on ${dest}." chmod "${mode}" "${dest}"
		return
	fi
	if [[ "${#TRACKED_FILES[@]}" -gt 0 ]]; then
		for f in "${TRACKED_FILES[@]}"; do [[ "${f}" == "${dest}" ]] && tracked=1 && break; done
	fi
	if [[ "${tracked}" -eq 0 ]]; then
		idx="${#TRACKED_FILES[@]}"
		if [[ -e "${dest}" ]]; then
			init_tmp_dir || return 1
			run_or_warn "Failed to back up existing APT file ${dest}." cp -p -- "${dest}" "${TMP_DIR}/backups/${idx}" || return 1
		fi
		TRACKED_FILES+=("${dest}")
	fi
	install_atomic_file "${src}" "${dest}" "${mode}"
}

commit_apt_changes() { TRACKED_FILES=(); }

rollback_apt_files() {
	local idx dest backup mode
	for ((idx = 0; idx < ${#TRACKED_FILES[@]}; idx++)); do
		dest="${TRACKED_FILES[idx]}"
		backup="${TMP_DIR}/backups/${idx}"
		if [[ -f "${backup}" ]]; then
			log_info "Restoring pre-existing APT file from backup: ${dest}"
			mode="$(file_mode "${backup}")"
			install_atomic_file "${backup}" "${dest}" "${mode:-0644}" || cp -pf -- "${backup}" "${dest}" || true
		elif [[ -e "${dest}" ]]; then
			log_info "Cleaning up newly created APT file: ${dest}"
			rm -f -- "${dest}" || true
		fi
	done
	commit_apt_changes
}

on_exit() {
	if [[ "${RESTORE_HOLD_ON_EXIT}" -eq 1 ]]; then
		run_apt_mark hold "${PACKAGE_NAME}" >/dev/null || log_warn "Failed to restore apt-mark hold on ${PACKAGE_NAME}."
		RESTORE_HOLD_ON_EXIT=0
	fi
	[[ "${INSTALL_SUCCEEDED}" -ne 1 ]] && rollback_apt_files
	[[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]] && rm -rf -- "${TMP_DIR}"
	return 0
}

# --- CLI Parsing & Validation ---
parse_args() {
	while [[ $# -gt 0 ]]; do
		case "$1" in
		"") shift ;;
		-h | --help)
			SHOW_HELP=1
			return 0
			;;
		--version=*)
			VERSION="${1#*=}"
			shift
			;;
		--version)
			[[ $# -ge 2 ]] || {
				log_warn "Missing value for argument: $1"
				return 1
			}
			VERSION="$2"
			shift 2
			;;
		*)
			log_warn "Unknown argument: $1"
			return 1
			;;
		esac
	done
	validate_config
}

require_match() {
	[[ "$2" =~ $3 ]] && return 0
	log_warn "Invalid $1 value '$2'."
	return 1
}
is_safe_url() {
	local bs="\\"
	[[ -n "$1" && ! "$1" =~ [[:space:]] && "$1" != *"#"* && "$1" != *"["* && "$1" != *"]"* && "$1" != *"${bs}"* ]]
}

validate_config() {
	[[ -z "${VERSION}" ]] || require_match "--version" "${VERSION}" '^[0-9][0-9A-Za-z.+~:-]*$' || return 1
	require_match "channel" "${CHANNEL}" '^[A-Za-z0-9._:/-]+$' || return 1
	require_match "APT suite" "${APT_SUITE}" '^[A-Za-z0-9._/+-]+$' || return 1
	require_match "APT component" "${APT_COMPONENT}" '^[A-Za-z0-9._/+-]+$' || return 1
	run_or_warn "Invalid APT repository URL '${APT_REPO_URL}'." is_safe_url "${APT_REPO_URL}" || return 1
	if [[ "${GPG_KEY_URL}" != https://* || "${#GPG_KEY_URL}" -le 8 ]] || ! is_safe_url "${GPG_KEY_URL}"; then
		log_warn "Invalid GPG key URL '${GPG_KEY_URL}'; must be an https:// URL."
		return 1
	fi
}

# --- Platform & Package Helpers ---
is_root() { [[ "${EUID:-$(id -u)}" -eq 0 ]]; }

check_platform() {
	local arch cmd msg=""
	arch="$(uname -m)"
	if [[ "${arch}" != "x86_64" ]]; then
		msg="Unsupported architecture '${arch}'; ${PACKAGE_NAME} requires x86_64. Skipping installation."
	else
		for cmd in apt-get apt-cache apt-mark dpkg dpkg-query systemctl; do
			command -v "${cmd}" >/dev/null 2>&1 || {
				msg="Required command '${cmd}' not found (unsupported OS); skipping ${PACKAGE_NAME} installation."
				break
			}
		done
		[[ -z "${msg}" ]] && ! is_root && msg="Root privileges are required to install ${PACKAGE_NAME}; skipping installation."
	fi
	if [[ -n "${msg}" ]]; then
		log_warn "${msg}"
		SKIP_INSTALL=1
	fi
}

run_apt_get() {
	DEBIAN_FRONTEND=noninteractive apt-get \
		-o DPkg::Lock::Timeout="${APT_LOCK_TIMEOUT}" -o Acquire::http::Timeout="${APT_HTTP_TIMEOUT}" \
		-o Acquire::https::Timeout="${APT_HTTP_TIMEOUT}" -o Acquire::Retries="${APT_RETRIES}" "$@"
}

apt_install() { run_apt_get -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install -y --no-install-recommends "$@"; }
apt_update_list() { run_apt_get -o Dir::Etc::sourcelist="sources.list.d/$(basename "$1")" -o Dir::Etc::sourceparts="-" -o APT::Get::List-Cleanup="0" update; }
run_apt_mark() { apt-mark -o DPkg::Lock::Timeout="${APT_LOCK_TIMEOUT}" "$@"; }
pkg_status() { dpkg-query -W -f='${Status}\n' "$1" 2>/dev/null || true; }
is_pkg_installed() { case "$(pkg_status "$1")" in "install ok installed" | "hold ok installed") return 0 ;; *) return 1 ;; esac }

get_package_state() {
	local query_output status ver
	PKG_INSTALLED=0 PKG_HELD=0 PKG_VERSION=""
	query_output="$(dpkg-query -W -f='${Status}\t${Version}\n' "${PACKAGE_NAME}" 2>/dev/null || true)"
	[[ -n "${query_output}" ]] || return 0
	IFS=$'\t' read -r status ver <<<"${query_output}"
	case "${status}" in
	"install ok installed") PKG_INSTALLED=1 PKG_VERSION="${ver}" ;;
	"hold ok installed") PKG_INSTALLED=1 PKG_HELD=1 PKG_VERSION="${ver}" ;;
	esac
}

has_apt_candidate() {
	local candidate
	candidate="$(LC_ALL=C apt-cache policy "$1" 2>/dev/null | awk '/^[ \t]*Candidate:/ && !found {print $2; found=1}')"
	[[ -n "${candidate}" && "${candidate}" != "(none)" ]]
}

is_pkg_version_available() {
	if [[ -z "${VERSION}" ]]; then
		has_apt_candidate "${PACKAGE_NAME}"
		return $?
	fi
	LC_ALL=C apt-cache madison "${PACKAGE_NAME}" 2>/dev/null | REQ_VERSION="${VERSION}" awk -F'|' '
		BEGIN { v = ENVIRON["REQ_VERSION"] }
		$3 ~ /Packages[ \t\r]*$/ { ver = $2; gsub(/^[ \t]+|[ \t]+$/, "", ver); if (ver == v) found = 1 }
		END { exit !found }
	'
}

# --- APT Source Discovery ---
apt_list_file_matches() {
	REQ_URI="$2" REQ_SUITE="$3" awk '
		BEGIN { matched = 0; req_uri = ENVIRON["REQ_URI"]; req_suite = ENVIRON["REQ_SUITE"]; sub(/\/+$/, "", req_uri) }
		{
			line = $0; sub(/#.*$/, "", line); gsub(/^[ \t\r]+|[ \t\r]+$/, "", line)
			if (line == "" || line !~ /^deb[ \t]/) next
			sub(/^deb[ \t]+/, "", line)
			if (line ~ /^\[/) { if (line !~ /^\[[^]]*\][ \t]+/) next; sub(/^\[[^]]*\][ \t]+/, "", line) }
			if (split(line, f, /[ \t]+/) < 2) next
			uri = f[1]; sub(/\/+$/, "", uri)
			if ((req_uri == "" || uri == req_uri) && f[2] == req_suite) matched = 1
		}
		END { exit (matched ? 0 : 1) }
	' "$1" 2>/dev/null
}

apt_sources_file_matches() {
	REQ_URI="$2" REQ_SUITE="$3" awk '
		function has_tok(str, target, strip_slash,    n, i, toks, t) {
			n = split(str, toks, /[ \t\r]+/)
			for (i = 1; i <= n; i++) {
				t = toks[i]; if (strip_slash) sub(/\/+$/, "", t)
				if (t == target) return 1
			}
			return 0
		}
		function check_stanza(    en) {
			en = tolower(f["enabled"]); gsub(/^[ \t\r]+|[ \t\r]+$/, "", en)
			if (!in_stanza || en == "no" || en == "false" || en == "0" || en == "off") return
			if (has_tok(f["types"], "deb", 0) && (req_uri == "" || has_tok(f["uris"], req_uri, 1)) && has_tok(f["suites"], req_suite, 0)) matched = 1
		}
		function reset_stanza() { in_stanza = 0; cur = ""; f["types"] = ""; f["uris"] = ""; f["suites"] = ""; f["enabled"] = "" }
		BEGIN { matched = 0; req_uri = ENVIRON["REQ_URI"]; req_suite = ENVIRON["REQ_SUITE"]; sub(/\/+$/, "", req_uri); reset_stanza() }
		/^[ \t]*#/ { next }
		/^[ \t\r]*$/ { check_stanza(); reset_stanza(); next }
		/^[ \t]+/ { if (cur != "") f[cur] = f[cur] " " $0; next }
		/^[A-Za-z0-9_-]+[ \t]*:/ {
			in_stanza = 1; c = index($0, ":"); cur = tolower(substr($0, 1, c - 1)); gsub(/[ \t]+$/, "", cur)
			f[cur] = substr($0, c + 1); next
		}
		END { check_stanza(); exit (matched ? 0 : 1) }
	' "$1" 2>/dev/null
}

find_existing_apt_sources() {
	local req_uri="$1" req_suite="$2" exclude_path="${3:-}" file base
	MATCHED_APT_SOURCES=()
	if [[ -f "${APT_SOURCES_LIST}" && "${APT_SOURCES_LIST}" != "${exclude_path}" ]]; then
		apt_list_file_matches "${APT_SOURCES_LIST}" "${req_uri}" "${req_suite}" && MATCHED_APT_SOURCES+=("${APT_SOURCES_LIST}")
	fi
	[[ -d "${APT_SOURCES_DIR}" ]] || return 0
	for file in "${APT_SOURCES_DIR}"/*; do
		[[ -f "${file}" && "${file}" != "${exclude_path}" ]] || continue
		base="$(basename "${file}")"
		if [[ "${base}" =~ ^[A-Za-z0-9_-][A-Za-z0-9_.-]*\.list$ ]]; then
			apt_list_file_matches "${file}" "${req_uri}" "${req_suite}" && MATCHED_APT_SOURCES+=("${file}")
		elif [[ "${base}" =~ ^[A-Za-z0-9_-][A-Za-z0-9_.-]*\.sources$ ]]; then
			apt_sources_file_matches "${file}" "${req_uri}" "${req_suite}" && MATCHED_APT_SOURCES+=("${file}")
		fi
	done
}

refresh_matched_apt_sources() {
	local file need_full_update=0 sourceparts_dir
	[[ "${#MATCHED_APT_SOURCES[@]}" -gt 0 ]] || return 0
	for file in "${MATCHED_APT_SOURCES[@]}"; do
		if [[ "${file}" == "${APT_SOURCES_DIR}/"*.list ]]; then
			apt_update_list "${file}" || return 1
		elif [[ "${file}" == "${APT_SOURCES_DIR}/"*.sources ]]; then
			init_tmp_dir || return 1
			if ! sourceparts_dir="$(mktemp -d "${TMP_DIR}/sourceparts.XXXXXX")" ||
				! ln -s -- "${file}" "${sourceparts_dir}/$(basename "${file}")"; then
				log_warn "Failed to stage scoped sourceparts directory for ${file}."
				return 1
			fi
			run_apt_get -o Dir::Etc::sourcelist="-" -o Dir::Etc::sourceparts="${sourceparts_dir}" -o APT::Get::List-Cleanup="0" update || return 1
		else
			need_full_update=1
		fi
	done
	[[ "${need_full_update}" -eq 0 ]] || run_apt_get update
}

# --- Package Installation ---
ensure_prereq_packages() {
	local need_key_tools="${1:-0}" missing_pkgs=()
	if [[ "${need_key_tools}" -eq 1 ]]; then
		command -v curl >/dev/null 2>&1 || missing_pkgs+=("curl")
		command -v gpg >/dev/null 2>&1 || missing_pkgs+=("gnupg")
	fi
	is_pkg_installed "ca-certificates" || missing_pkgs+=("ca-certificates")
	[[ "${#missing_pkgs[@]}" -eq 0 ]] && return 0

	log_info "Installing missing prerequisite packages: ${missing_pkgs[*]}"
	run_apt_get update || log_warn "apt-get update failed while installing prerequisites (${missing_pkgs[*]}); continuing with cached metadata."
	run_or_warn "Failed to install prerequisite packages: ${missing_pkgs[*]}" apt_install "${missing_pkgs[@]}"
}

download_and_dearmor_key() {
	local url="$1" dest_keyring="$2" key_base raw_key dearmored_key
	init_tmp_dir || return 1
	ensure_prereq_packages 1 || return 1
	key_base="$(basename "${dest_keyring}")"
	raw_key="${TMP_DIR}/${key_base}.raw"
	dearmored_key="${TMP_DIR}/${key_base}.dearmored"

	run_or_warn "Failed to download GPG key from ${url}." \
		curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --tlsv1.2 \
		--retry 2 --retry-delay 2 --retry-max-time 25 --connect-timeout 10 --max-time 20 \
		--output "${raw_key}" -- "${url}" || return 1
	if ! GNUPGHOME="${TMP_DIR}/gnupg" gpg --batch --yes --dearmor --output "${dearmored_key}" "${raw_key}" || [[ ! -s "${dearmored_key}" ]]; then
		log_warn "Failed to dearmor GPG key from ${url}."
		return 1
	fi
	install_tracked_file "${dearmored_key}" "${dest_keyring}" 0644
}

ensure_apt_repo() {
	local label="$1" match_uri="$2" gpg_url="$3" keyring="$4" repo_url="$5" suite="$6" component="$7" list_file="$8" list_tmp
	ensure_prereq_packages 0 || return 1
	find_existing_apt_sources "${match_uri}" "${suite}" "${list_file}"
	if [[ "${#MATCHED_APT_SOURCES[@]}" -gt 0 ]]; then
		log_info "Reusing existing active APT source(s) for ${label}: ${MATCHED_APT_SOURCES[*]}."
		refresh_matched_apt_sources || log_warn "apt-get update failed while refreshing existing ${label} source(s); continuing with cached metadata."
		return 0
	fi
	log_info "Configuring ${label} APT repository (${repo_url} ${suite} ${component})."
	init_tmp_dir || return 1
	download_and_dearmor_key "${gpg_url}" "${keyring}" || return 1
	list_tmp="${TMP_DIR}/$(basename "${list_file}")"
	printf 'deb [signed-by=%s] %s %s %s\n' "${keyring}" "${repo_url}" "${suite}" "${component}" >"${list_tmp}" || {
		log_warn "Failed to stage ${list_file}."
		return 1
	}
	install_tracked_file "${list_tmp}" "${list_file}" 0644 || return 1
	run_or_warn "Failed to update APT metadata for ${list_file}." apt_update_list "${list_file}"
}

ensure_ar_transport() {
	[[ "${APT_REPO_URL}" == ar+* ]] || return 0
	is_pkg_installed "${AR_TRANSPORT_PACKAGE}" && return 0
	ensure_prereq_packages 0 || return 1
	if has_apt_candidate "${AR_TRANSPORT_PACKAGE}"; then
		log_info "Installing ${AR_TRANSPORT_PACKAGE} from existing APT repositories."
		apt_install "${AR_TRANSPORT_PACKAGE}" && return 0
		log_info "Initial install of ${AR_TRANSPORT_PACKAGE} failed; refreshing repository metadata and retrying once."
	fi
	ensure_apt_repo "${AR_TRANSPORT_SUITE}" "" "https://packages.cloud.google.com/apt/doc/apt-key.gpg" "${AR_TRANSPORT_KEYRING}" \
		"https://packages.cloud.google.com/apt" "${AR_TRANSPORT_SUITE}" "main" "${AR_TRANSPORT_LIST}" || return 1
	run_or_warn "Failed to install ${AR_TRANSPORT_PACKAGE}." apt_install "${AR_TRANSPORT_PACKAGE}"
}

ensure_diagon_package() {
	get_package_state
	if [[ "${PKG_INSTALLED}" -eq 1 && (-z "${VERSION}" || "${PKG_VERSION}" == "${VERSION}") ]]; then
		if [[ -z "${VERSION}" && "${PKG_HELD}" -eq 1 ]]; then
			log_info "Unholding previously pinned ${PACKAGE_NAME} (${PKG_VERSION})."
			run_apt_mark unhold "${PACKAGE_NAME}" >/dev/null || log_warn "Failed to unhold ${PACKAGE_NAME}; continuing."
		elif [[ -n "${VERSION}" && "${PKG_HELD}" -ne 1 ]]; then
			log_info "Pinning ${PACKAGE_NAME} at version ${VERSION} via apt-mark hold."
			run_apt_mark hold "${PACKAGE_NAME}" >/dev/null || log_warn "Failed to pin ${PACKAGE_NAME} with apt-mark hold; continuing."
		fi
		log_info "${PACKAGE_NAME} (${PKG_VERSION}) is already installed; skipping APT installation."
		PACKAGE_CHANGED=0
		return 0
	fi

	init_tmp_dir || return 1
	ensure_ar_transport || return 1
	ensure_apt_repo "Google Cloud ML Diagnostics" "${APT_REPO_URL%/}" "${GPG_KEY_URL}" "${DIAGON_KEYRING}" \
		"${APT_REPO_URL}" "${APT_SUITE}" "${APT_COMPONENT}" "${DIAGON_LIST}" || return 1
	run_or_warn "Package ${PACKAGE_NAME}${VERSION:+=${VERSION}} was not found in APT repositories." is_pkg_version_available || return 1
	local pkg_spec="${PACKAGE_NAME}${VERSION:+=${VERSION}}" apt_args=("${PACKAGE_NAME}")
	[[ -n "${VERSION}" ]] && apt_args=(--allow-downgrades "${pkg_spec}")
	if [[ "${PKG_HELD}" -eq 1 ]]; then
		log_info "Temporarily unholding ${PACKAGE_NAME} to install ${pkg_spec}."
		run_or_warn "Failed to unhold ${PACKAGE_NAME}." run_apt_mark unhold "${PACKAGE_NAME}" >/dev/null || return 1
		RESTORE_HOLD_ON_EXIT=1
	fi
	log_info "Installing ${pkg_spec}."
	if ! apt_install "${apt_args[@]}"; then
		if ((SECONDS >= FULL_UPDATE_RETRY_DEADLINE_SECS)); then
			log_warn "Failed to install ${pkg_spec}; skipping full APT cache refresh retry after ${SECONDS}s elapsed."
			return 1
		fi
		log_info "Initial install of ${pkg_spec} failed; refreshing APT cache for dependency resolution and retrying once."
		run_apt_get update || log_warn "apt-get update failed during dependency cache refresh; continuing with cached metadata."
		run_or_warn "Failed to install ${pkg_spec}." apt_install "${apt_args[@]}" || return 1
	fi
	# Commit APT files once installed so later hold/systemd errors keep them.
	commit_apt_changes
	if [[ -n "${VERSION}" ]]; then
		log_info "Pinning ${PACKAGE_NAME} at version ${VERSION} via apt-mark hold."
		run_apt_mark hold "${PACKAGE_NAME}" >/dev/null || log_warn "Failed to hold ${PACKAGE_NAME} at version ${VERSION}; continuing."
	elif [[ "$(pkg_status "${PACKAGE_NAME}")" == "hold ok installed" ]]; then
		run_apt_mark unhold "${PACKAGE_NAME}" >/dev/null || log_warn "Failed to unhold ${PACKAGE_NAME} after install."
	fi
	RESTORE_HOLD_ON_EXIT=0
	PACKAGE_CHANGED=1
}

# --- Systemd Service ---
configure_and_start_service() {
	run_or_warn "Expected daemon binary ${BINARY_PATH} does not exist or is not executable." test -x "${BINARY_PATH}" || return 1
	init_tmp_dir || return 1

	local unit_tmp="${TMP_DIR}/${SERVICE_NAME}" unit_changed=0 action=""
	if ! cat >"${unit_tmp}" <<EOF
[Unit]
Description=Google Cloud ML Diagnostics Daemon
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
Type=simple
ExecStart=${BINARY_PATH} -standalone=true -channel=${CHANNEL} -alsologtostderr=true
Restart=on-failure
RestartSec=15
OOMScoreAdjust=500

[Install]
WantedBy=multi-user.target
EOF
	then
		log_warn "Failed to stage ${SERVICE_FILE}."
		return 1
	fi

	if [[ ! -f "${SERVICE_FILE}" ]] || ! cmp -s "${unit_tmp}" "${SERVICE_FILE}"; then
		log_info "Writing systemd unit file ${SERVICE_FILE}."
		install_atomic_file "${unit_tmp}" "${SERVICE_FILE}" 0644 || return 1
		unit_changed=1
	fi
	[[ "${unit_changed}" -eq 0 ]] || run_or_warn "Failed to reload systemd daemon." systemctl daemon-reload || return 1
	systemctl is-enabled --quiet "${SERVICE_NAME}" || run_or_warn "Failed to enable ${SERVICE_NAME}." systemctl enable "${SERVICE_NAME}" || return 1

	if [[ "${PACKAGE_CHANGED}" -eq 1 || "${unit_changed}" -eq 1 ]]; then
		local verb="Starting"
		systemctl is-active --quiet "${SERVICE_NAME}" && verb="Restarting"
		log_info "${verb} ${SERVICE_NAME}."
		action="restart"
	elif ! systemctl is-active --quiet "${SERVICE_NAME}"; then
		log_info "Starting ${SERVICE_NAME}."
		action="start"
	fi
	[[ -z "${action}" ]] || run_or_warn "Failed to ${action} ${SERVICE_NAME}." systemctl "${action}" "${SERVICE_NAME}" || return 1
	run_or_warn "${SERVICE_NAME} is not active after startup." systemctl is-active --quiet "${SERVICE_NAME}" || return 1
	log_info "${SERVICE_NAME} is enabled and active."
}

# --- Entrypoint ---
main() {
	trap on_exit EXIT
	trap 'log_warn "Interrupted by signal; cleaning up (fail-open)."; exit 0' INT TERM
	if parse_args "$@"; then
		if [[ "${SHOW_HELP}" -eq 1 ]]; then
			usage
			INSTALL_SUCCEEDED=1
			return 0
		fi
		if check_platform && { [[ "${SKIP_INSTALL}" -eq 1 ]] || { ensure_diagon_package && configure_and_start_service; }; }; then
			INSTALL_SUCCEEDED=1
			return 0
		fi
	fi
	log_warn "Google Cloud ML Diagnostics daemon installation did not complete; continuing VM startup (fail-open)."
	return 0
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
