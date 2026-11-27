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

# Startup script to prepare a new VM to host the web application for the
# Multi-platform HPC Application System

# based on a CentOS 8 system

#set -v

# Obtain metadata of this server
#
GCP_PROJECT=$(curl --silent --show-error http://metadata.google.internal/computeMetadata/v1/project/project-id -H "Metadata-Flavor: Google")
SERVER_IP_ADDRESS=$(curl --silent --fail http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip -H "Metadata-Flavor: Google")
if [ -z "${SERVER_IP_ADDRESS}" ]; then
	# No public IP.  Fall back to internal
	SERVER_IP_ADDRESS=$(curl --silent --fail http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ip -H "Metadata-Flavor: Google")
fi
SERVER_HOSTNAME=$(curl --silent --fail http://metadata/computeMetadata/v1/instance/attributes/hostname -H "Metadata-Flavor: Google")
config_bucket=$(curl --silent --show-error http://metadata/computeMetadata/v1/instance/attributes/webserver-config-bucket -H "Metadata-Flavor: Google")
state_bucket=$(curl --silent --show-error http://metadata/computeMetadata/v1/instance/attributes/terraform-state-bucket -H "Metadata-Flavor: Google")
c2_topic=$(curl --silent --show-error http://metadata/computeMetadata/v1/instance/attributes/ghpcfe-c2-topic -H "Metadata-Flavor: Google")
deploy_mode=$(curl --silent --show-error http://metadata/computeMetadata/v1/instance/attributes/deploy_mode -H "Metadata-Flavor: Google")
deployment_name=$(curl --silent --show-error http://metadata/computeMetadata/v1/instance/attributes/deployment_name -H "Metadata-Flavor: Google")

# The TKFE code, and the markers of the first boot's progress. The first boot
# writes first-boot-started before it fetches the code, first-boot-fetched
# once the code is fetched, and first-boot-complete once the server is
# installed and running. A server with the code and the first two markers
# only stopped part-way through its first boot and cannot start; one with no
# markers was installed by an earlier TKFE version.
tkfe_code_dir=/opt/gcluster/cluster-toolkit
ofe_state_dir=/var/lib/ofe-server
first_boot_started=${ofe_state_dir}/first-boot-started
first_boot_fetched=${ofe_state_dir}/first-boot-fetched
first_boot_complete=${ofe_state_dir}/first-boot-complete

# A first boot interrupted while it fetched the code (a reboot during the
# clone, say) leaves part of it and only the started marker. Nothing else is
# installed yet, so remove the partial copy and run the first boot again.
if [[ -f ${first_boot_started} && ! -f ${first_boot_fetched} &&
	! -f ${first_boot_complete} && -d ${tkfe_code_dir} ]]; then
	printf "A previous first boot stopped while fetching the TKFE code; removing the partial copy and starting again\n"
	rm -rf "${tkfe_code_dir:?}"
fi

# Exit if deployment already exists to stop startup script running on reboots
#
if [[ -d ${tkfe_code_dir} ]]; then
	printf "It appears gcluster has already been deployed.\n"
	# An installed server never reads its configuration again, so every
	# version of the configuration secret that is still enabled is disabled:
	# one an earlier TKFE version left enabled, or one a later Terraform apply
	# added. A failure is reported, not fatal: the server is installed and
	# must still start.
	installed_config_secret=$(curl --silent --fail http://metadata/computeMetadata/v1/instance/attributes/ofe-server-config-secret -H "Metadata-Flavor: Google")
	# A first boot that stopped after fetching the TKFE code but before it
	# finished installing leaves a server that cannot start. Stop without
	# disabling any version or preparing credentials, so a new VM can still
	# read its configuration. The web server may already be enabled (a boot
	# reset after it started it), so stop and disable it: a half-installed
	# server must not serve.
	if [[ ! -f ${first_boot_complete} && -f ${first_boot_started} ]]; then
		if systemctl disable --now gcluster.service; then
			web_server="the web server has been stopped and disabled"
		else
			printf "ERROR: could not stop the web server; run: systemctl disable --now gcluster.service\n"
			web_server="the web server could not be stopped (see above)"
		fi
		if [[ -n ${installed_config_secret} ]]; then
			printf "ERROR: the first boot did not finish installing TKFE; this server cannot start, and %s. Recreate the server VM; the enabled versions of secret %s stay enabled so the new VM can read its configuration\n" \
				"${web_server}" "${installed_config_secret}"
		else
			printf "ERROR: the first boot did not finish installing TKFE; this server cannot start, and %s. Recreate the server VM\n" \
				"${web_server}"
		fi
		exit 1
	fi
	if [[ -n ${installed_config_secret} ]]; then
		if enabled_config_versions=$(gcloud secrets versions list "${installed_config_secret}" \
			--project="${GCP_PROJECT}" --filter="state:ENABLED" --format="value(name.basename())"); then
			for config_version in ${enabled_config_versions}; do
				if gcloud secrets versions disable "${config_version}" \
					--secret="${installed_config_secret}" --project="${GCP_PROJECT}" --quiet; then
					printf "Disabled version %s of secret %s\n" "${config_version}" "${installed_config_secret}"
				else
					printf "WARNING: could not disable version %s of secret %s, which holds the admin password; disable it by hand\n" "${config_version}" "${installed_config_secret}"
				fi
			done
		else
			printf "WARNING: could not list the versions of secret %s, which holds the admin password; disable its enabled versions by hand\n" "${installed_config_secret}"
		fi
	fi
	# Stored credentials are prepared on every start (the command is safe to
	# run again), so a server whose code was upgraded encrypts its stored
	# keys and registers its provisioning account too. Older code has no
	# such command.
	if [[ -f /opt/gcluster/cluster-toolkit/community/front-end/ofe/website/ghpcfe/management/commands/ofe_bootstrap_credentials.py ]]; then
		printf "Preparing stored credentials...\n"
		sudo su - gcluster -c /bin/bash <<'EOF'
  source /opt/gcluster/django-env/bin/activate
  cd /opt/gcluster/cluster-toolkit/community/front-end/ofe/website
  python manage.py ofe_bootstrap_credentials || printf "ERROR: preparing stored credentials failed; see the messages above. TKFE shows the recorded errors to administrators until a start succeeds\n"
EOF
	fi
	printf "Exiting...\n"
	exit 0
fi

printf "\n############## Setting SELinux to Permissive ###############\n"
setenforce 0
sed -i -e 's/SELINUX=enforcing/SELINUX=permissive/g' /etc/selinux/config

printf "####################\n#### Installing required packages\n####################\n"
dnf install -y epel-release
dnf update -y --security
dnf update -y expat
dnf config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo

dnf install -y terraform
dnf install --best -y google-cloud-cli nano make gcc python3.12-devel unzip git \
	rsync wget nginx bind-utils policycoreutils-python-utils \
	packer supervisor python3-certbot-nginx jq
curl --silent --show-error --location https://github.com/mikefarah/yq/releases/download/v4.13.4/yq_linux_amd64 --output /usr/local/bin/yq
chmod +x /usr/local/bin/yq
curl --silent --show-error --location https://github.com/koalaman/shellcheck/releases/download/stable/shellcheck-stable.linux.x86_64.tar.xz --output /tmp/shellcheck.tar.xz
tar xfa /tmp/shellcheck.tar.xz --strip=1 --directory /usr/local/bin

# Install Grafana
curl -sSL -o gpg.key https://rpm.grafana.com/gpg.key
rpm --import gpg.key

tee /etc/yum.repos.d/grafana.repo <<EOL
[grafana]
name=grafana
baseurl=https://rpm.grafana.com
repo_gpgcheck=1
enabled=1
gpgcheck=1
gpgkey=https://rpm.grafana.com/gpg.key
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
exclude=*beta*
EOL

dnf install -y grafana

# Build and install latest SQLite for Django 5 compatibility
# dnf/yum only has 3.26.0 which causes error:
# "django.db.utils.NotSupportedError: SQLite 3.31 or later is required (found 3.26.0)."
install_sqlite() {
	local src_dir="/usr/local/src"
	local tarball="sqlite-autoconf-3490100.tar.gz"
	local url="https://www.sqlite.org/2025/${tarball}"

	cd "$src_dir" || {
		echo "ERROR: Could not change directory to $src_dir"
		exit 1
	}

	if ! wget "$url"; then
		echo "ERROR: Failed to download SQLite from $url"
		exit 1
	fi

	tar xzf sqlite-autoconf-3490100.tar.gz
	cd sqlite-autoconf-3490100 || exit 1
	./configure --prefix=/usr/local
	make
	sudo make install
	echo "/usr/local/lib" | sudo tee /etc/ld.so.conf.d/sqlite3.conf
	sudo ldconfig
}

install_sqlite

python3.12 -m ensurepip --upgrade

pip3.12 install --require-hashes -r "$(dirname "$0")/requirements.txt"

# Set Python3.12 as default Python3
echo '2' | update-alternatives --config python3
# Read the configuration file (the admin user's details and the other
# first-boot settings) from its Secret Manager secret, which only this server's
# account may read. Only root can read the copy, which is removed once read.
# The version read stays enabled until the server is installed and running
# (see below). A boot that stops before the TKFE code is fetched, or whose
# fetch fails, leaves no code, so the next boot tries again. One that stops
# later leaves the server half-installed: later boots stop with an error (see
# the top of this script) and leave the version enabled, and the server VM
# must be recreated.
#
config_secret=$(curl --silent --fail http://metadata/computeMetadata/v1/instance/attributes/ofe-server-config-secret -H "Metadata-Flavor: Google")
if [[ -z ${config_secret} ]]; then
	printf "ERROR: the instance metadata key ofe-server-config-secret is not set\n"
	exit 1
fi
# The newest version is resolved to its number first, so the version read is
# the version disabled once the server is installed.
if ! config_version=$(gcloud secrets versions describe latest \
	--secret="${config_secret}" --project="${GCP_PROJECT}" --format="value(name.basename())") ||
	[[ -z ${config_version} ]]; then
	printf "ERROR: could not find the newest version of secret %s\n" "${config_secret}"
	exit 1
fi
rm -f /tmp/config
if ! (umask 077 && gcloud secrets versions access "${config_version}" \
	--secret="${config_secret}" --project="${GCP_PROJECT}" >/tmp/config); then
	printf "ERROR: could not read the server configuration from secret %s\n" "${config_secret}"
	printf "A completed install disables the version it read; if version %s is disabled, enable it again with: gcloud secrets versions enable %s --secret=%s --project=%s\n" \
		"${config_version}" "${config_version}" "${config_secret}" "${GCP_PROJECT}"
	rm -f /tmp/config
	exit 1
fi

# Load configurations
#
DJANGO_USERNAME=$(/usr/local/bin/yq e '.django_username' /tmp/config)
DJANGO_PASSWORD=$(/usr/local/bin/yq e '.django_password' /tmp/config)
DJANGO_EMAIL=$(/usr/local/bin/yq e '.django_email' /tmp/config)
GOOGLE_CLIENT_ID=$(/usr/local/bin/yq e '.google_client_id' /tmp/config)
GOOGLE_CLIENT_SECRET=$(/usr/local/bin/yq e '.google_client_secret' /tmp/config)
repo_fork=$(/usr/local/bin/yq e '.git_fork' /tmp/config)
repo_branch=$(/usr/local/bin/yq e '.git_branch' /tmp/config)
# 'yq' does not handle multi-line string properly, need to restore the correct key format
rm -f /tmp/config

printf "\n####################\n#### Creating firewall & SELinux rules\n####################\n"
printf "Adding rule for port 22 (ssh): "
firewall-cmd --permanent --add-port=22/tcp
printf "Adding rule for port 80: "
firewall-cmd --permanent --add-port=80/tcp
printf "Adding rule for port 443: "
firewall-cmd --permanent --add-port=443/tcp
printf "Reloading firewall: "
firewall-cmd --reload
setsebool httpd_can_network_connect on -P

# Record that the first boot has started, before the code is fetched: a boot
# that stops part-way once the code is there leaves a server that cannot
# start, and later boots find this marker without first-boot-complete.
# Without it a half-installed server would look like one from an earlier
# TKFE version, so stop: nothing is installed yet, and a restart tries again.
if ! mkdir -p "${ofe_state_dir}" ||
	! rm -f "${first_boot_complete}" "${first_boot_fetched}" ||
	! touch "${first_boot_started}"; then
	printf "ERROR: could not write %s; nothing has been installed, restart the server VM to try again\n" "${first_boot_started}"
	exit 1
fi

printf "\n####################\n#### Create gcluster user & create deployment key\n####################\n"
printf "Adding gcluster user...\n"
#
# The gcluster user may already have been created by the google_guest_agent
#
# - This agent picks up any users identified in the GCP project ssh-keys
#   metadata and automatically creates user accounts (and dirs in /home) on
#   the VM.
#
# - The gcluster ssh-key will only exist if the admin has used it with gcloud,
#   however, this is a gotcha - we need the gcluster $HOME be /opt/gcluster,
#   otherwise the webserver will not deploy.
#
# - Need to make sure gcluster account as we need it to be.  Simplest thing to
#   do, is remove the account if it already exists.
#
# - Create the account as required, as a system account
#   (Note: if google_guest_agent creates the account, it is not a system account
#    so it is better to recreate it as one, rather than just usermod it).
#
if id gcluster >/dev/null 2>&1; then
	userdel -r gcluster 2>/dev/null
fi
useradd -r -m -d /opt/gcluster gcluster

if [ "${deploy_mode}" == "git" ]; then
	fetch_hpc_toolkit="git clone -b \"${repo_branch}\" https://github.com/${repo_fork}/cluster-toolkit.git"

elif [ "${deploy_mode}" == "tarball" ]; then
	printf "\n####################\n#### Download web application files\n####################\n"
	gcloud storage cp "gs://${config_bucket}/webserver/deployment.tar.gz" /tmp/deployment.tar.gz

	fetch_hpc_toolkit="tar xfz /tmp/deployment.tar.gz"
fi

# Clean up anything we may have missed
#
chown gcluster:gcluster -R /opt/gcluster

# Run the following as 'gcluster' user
#
sudo su - gcluster -c /bin/bash <<EOF
  set -e
  cd /opt/gcluster
  ${fetch_hpc_toolkit}
EOF
fetch_rc=$?
# The code is all there: from here on, a boot that stops leaves a server that
# cannot start (see the top of this script). Without the marker a later boot
# would take the code for a partial copy, so treat that as a failed fetch.
if [[ ${fetch_rc} -eq 0 ]] && ! touch "${first_boot_fetched}"; then
	printf "ERROR: could not write %s\n" "${first_boot_fetched}"
	fetch_rc=1
fi
# A failed fetch can leave part of the code (a partial extract); remove it so
# the next boot runs the first boot again instead of finding an install.
if [[ ${fetch_rc} -ne 0 ]]; then
	printf "ERROR: fetching the TKFE code failed\n"
	rm -rf "${tkfe_code_dir:?}"
fi

# Install go version specified in go.mod file
#
# Note: go.mod doesn't reference minor version so we need to capture the latest
GO_MAJOR_VERSION=$(awk '/^go/ {print $2}' "/opt/gcluster/cluster-toolkit/go.mod")
GO_API_RESPONSE=$(curl --silent "https://go.dev/dl/?mode=json&include=all")
GO_VERSION=$(echo "$GO_API_RESPONSE" | jq -r --arg major "go$GO_MAJOR_VERSION" '.[] | select(.version | startswith($major)).version' | sort -V | tail -n 1)
GO_DOWNLOAD_URL="https://golang.org/dl/${GO_VERSION}.linux-amd64.tar.gz"
curl --silent --show-error --location "${GO_DOWNLOAD_URL}" --output "/tmp/${GO_VERSION}.linux-amd64.tar.gz"
rm -rf /usr/local/go && tar -C /usr/local -xzf "/tmp/${GO_VERSION}.linux-amd64.tar.gz"

# Add path entry for Go binaries to bashrc for all users (only works on future logins)
# (once: a first boot run again must not add it twice)
# shellcheck disable=SC2016
go_path_line='export PATH=$PATH:/usr/local/go/bin:~/go/bin'
grep -qxF "${go_path_line}" /etc/bashrc || printf '%s\n' "${go_path_line}" >>/etc/bashrc

# Any failed step fails the set-up, except preparing stored credentials,
# whose errors TKFE shows to administrators.
sudo su - gcluster -c /bin/bash <<EOF
  set -e
  cd /opt/gcluster/cluster-toolkit/community/front-end/ofe

  printf "\nDownloading Frontend dependencies...\n"
  mkdir dependencies
  pushd dependencies
  git clone -b v0.21.0 --depth 1 https://github.com/spack/spack.git
  printf "\npre-generating Spack package list\n"
  ./spack/bin/spack list > /dev/null
  popd

  printf "\nDownloading ghpc dependencies...\n"
  go install github.com/terraform-docs/terraform-docs@v0.16.0
  go install github.com/google/addlicense@v1.2.0
  pushd /opt/gcluster/cluster-toolkit
  make
  popd

  printf "\nEstablishing django environment..."
  python3.12 -m venv /opt/gcluster/django-env
  source /opt/gcluster/django-env/bin/activate
  printf "\nInstalling pip...\n"
  pip install --require-hashes -r /opt/gcluster/cluster-toolkit/community/front-end/ofe/infrastructure_files/gcs_bucket/webserver/django_requirements.txt
  printf "\nInstalling pip requirements...\n"
  pip install --require-hashes -r /opt/gcluster/cluster-toolkit/community/front-end/ofe/requirements.txt

  printf "Generating configuration file for backend..."
  echo "config:" > configuration.yaml
  echo "  server:" >> configuration.yaml
  echo "    host_type: \"GCP\"" >> configuration.yaml
  echo "    gcp_project: \"$GCP_PROJECT\"" >> configuration.yaml
  echo "    gcs_bucket: \"${config_bucket}\"" >> configuration.yaml
  echo "    terraform_state_bucket: \"${state_bucket}\"" >> configuration.yaml
  echo "    c2_topic: \"${c2_topic}\"" >> configuration.yaml
  echo "    deployment_name: \"${deployment_name}\"" >> configuration.yaml

  printf "\nInitalising Django environments...\n"
  mkdir /opt/gcluster/run
  pushd website
  python manage.py makemigrations ghpcfe
  python manage.py migrate
  printf "\nCreating django super user..."
  DJANGO_SUPERUSER_PASSWORD=$DJANGO_PASSWORD python manage.py createsuperuser --username $DJANGO_USERNAME --email $DJANGO_EMAIL --noinput
  printf "\nInitialise Django db"
  python manage.py custom_setup_command $GOOGLE_CLIENT_ID $GOOGLE_CLIENT_SECRET ${SERVER_HOSTNAME:-${SERVER_IP_ADDRESS}}
  printf "\nPreparing stored credentials...\n"
  # The database key in Secret Manager, and the provisioning account's
  # credential, owned by the admin user created above
  python manage.py ofe_bootstrap_credentials || printf "ERROR: preparing stored credentials failed; see the messages above. TKFE shows the recorded errors to administrators until a start succeeds\n"
  printf "\nSet up static contents..."
  python manage.py collectstatic --noinput
  python manage.py seed_workbench_presets
  popd

  printf "\nUpdating nginx config...\n"
  if [ -n "${SERVER_HOSTNAME}" ] ; then
    sed "s/SERVER_NAME/$SERVER_HOSTNAME/g" -i website/nginx.conf
  else
    # No server name set, remove the entry
    sed "/SERVER_NAME/d" -i website/nginx.conf
  fi

EOF
setup_rc=$?

# Tweak Grafana settings
#
cat <<EOL >/etc/grafana/grafana.ini
[server]
serve_from_sub_path = true
root_url = %(protocol)s://%(domain)s:%(http_port)s/grafana/
[auth.proxy]
enabled = true
whitelist = 127.0.0.1
header_property = email
EOL

printf "Creating supervisord service..."
echo "[program:gcluster-uvicorn-background]
process_name=%(program_name)s_%(process_num)02d
directory=/opt/gcluster/cluster-toolkit/community/front-end/ofe/website
command=/opt/gcluster/django-env/bin/uvicorn website.asgi:application --reload --host 127.0.0.1 --port 8001
autostart=true
autorestart=true
user=gcluster
redirect_stderr=true
environment=HOME=/opt/gcluster
stdout_logfile=/opt/gcluster/run/supvisor.log" >/etc/supervisord.d/gcluster.ini

printf "Creating systemd service..."
echo "[Unit]
Description=GCluster: The GCP HPC Cluster deployment tool
Requires=supervisord.service grafana-server.service
After=supervisord.service grafana-server.service


[Service]
Type=forking
ExecStart=/usr/sbin/nginx -p /opt/gcluster/run/ -c /opt/gcluster/cluster-toolkit/community/front-end/ofe/website/nginx.conf
ExecStop=/usr/sbin/nginx -p /opt/gcluster/run/ -c /opt/gcluster/cluster-toolkit/community/front-end/ofe/website/nginx.conf -s stop
PIDFile=/opt/gcluster/run/nginx.pid
Restart=no


[Install]
WantedBy=default.target" >/etc/systemd/system/gcluster.service

printf "Reloading systemd and starting service..."
systemctl daemon-reload
systemctl enable gcluster.service
systemctl start gcluster.service
systemctl status gcluster.service

# Once the server is installed and running it never reads its configuration
# again: record that the first boot is complete, then disable the version read
# above, so the admin password cannot be read again. The marker comes first:
# a boot stopped between the two leaves a complete server, and each later
# start disables every version still enabled. The script does not stop
# at a failed step, so the install is complete only if the code was fetched,
# the set-up succeeded, the server (nginx) is running and so is the TKFE
# application under supervisord. Otherwise the version stays enabled: after a
# failed fetch there is no code, so restarting the VM runs the first boot
# again; after a later failure the server must be recreated. Either way the
# web server, the only public listener, is stopped and disabled, and the
# script stops with an error, so a half-installed server does not serve. A failed disable of the version is
# reported, not fatal: the install is complete, and each later start disables
# every version still enabled.
#
# supervisord starts the application in the background and reports it RUNNING
# only once it has stayed up for its startsecs, so it is polled, every
# app_wait_interval seconds and at most app_wait_polls times (60 s of waiting
# between polls, plus at most 5 s for each answer). RUNNING alone is not
# enough: the program runs uvicorn with --reload, whose reloader stays
# RUNNING when the application fails to import, so the application must also
# answer on its port (8001, see the supervisord program above); any HTTP
# answer means Django loaded, while a refused or closed connection does not.
# The exit status of supervisorctl status differs between supervisor
# versions, so its output is read instead: every process of the program must
# be RUNNING. Only standard output is read (it carries socket and group
# errors too); a warning on standard error goes to the log and does not
# count, and no output counts as not running.
app_wait_polls=30
app_wait_interval=2
tkfe_app_running() {
	local poll
	for ((poll = 1; poll <= app_wait_polls; poll++)); do
		if supervisorctl status 'gcluster-uvicorn-background:*' |
			awk 'NF { n++; if ($2 != "RUNNING") bad = 1 } END { exit !(n > 0 && !bad) }' &&
			curl --silent --output /dev/null --max-time 5 http://127.0.0.1:8001/; then
			return 0
		fi
		if ((poll < app_wait_polls)); then
			sleep "${app_wait_interval}"
		fi
	done
	return 1
}
install_failure=""
if [[ ${fetch_rc} -ne 0 ]]; then
	install_failure="fetching the TKFE code failed"
elif [[ ${setup_rc} -ne 0 ]]; then
	install_failure="setting up TKFE failed"
elif ! systemctl is-active --quiet gcluster.service; then
	install_failure="the gcluster service is not running"
elif ! tkfe_app_running; then
	install_failure="the TKFE application is not running"
fi
if [[ -z ${install_failure} ]]; then
	if ! touch "${first_boot_complete}"; then
		# Not disabled: later starts take this server for a half-installed one
		# until the marker exists, and say its versions stay enabled
		printf "ERROR: could not write %s, which records that the first boot is complete, so version %s of secret %s stays enabled; run: sudo touch %s, then restart the server VM\n" \
			"${first_boot_complete}" "${config_version}" "${config_secret}" "${first_boot_complete}"
	elif ! gcloud secrets versions disable "${config_version}" \
		--secret="${config_secret}" --project="${GCP_PROJECT}" --quiet; then
		printf "ERROR: could not disable version %s of secret %s, which holds the admin password; the next start disables it, or run: gcloud secrets versions disable %s --secret=%s --project=%s\n" \
			"${config_version}" "${config_secret}" "${config_version}" "${config_secret}" "${GCP_PROJECT}"
	fi
else
	if systemctl disable --now gcluster.service; then
		web_server="the web server has been stopped and disabled"
	else
		printf "ERROR: could not stop the web server; run: systemctl disable --now gcluster.service\n"
		web_server="the web server could not be stopped (see above)"
	fi
	if [[ ! -d ${tkfe_code_dir} ]]; then
		printf "ERROR: the first boot did not finish installing TKFE (%s); version %s of secret %s stays enabled, and %s. No TKFE code was installed, so restarting the server VM tries again\n" \
			"${install_failure}" "${config_version}" "${config_secret}" "${web_server}"
	else
		printf "ERROR: the first boot did not finish installing TKFE (%s); version %s of secret %s stays enabled, and %s. Recreate the server VM\n" \
			"${install_failure}" "${config_version}" "${config_secret}" "${web_server}"
	fi
	# Nothing after this block runs on a failed install: certbot's nginx
	# plugin could start nginx again, and the Grafana set-up needs a running
	# server. The failure also shows in the startup script's status.
	exit 1
fi

# Now that Grafana and Django are running, initialize integration
#
sudo su - gcluster -c /bin/bash <<EOF
  source /opt/gcluster/django-env/bin/activate
  cd /opt/gcluster/cluster-toolkit/community/front-end/ofe/website
  python manage.py setup_grafana "${DJANGO_EMAIL}"
EOF

# If we have a hostname, configure for TLS
#
if [ -n "${SERVER_HOSTNAME}" ]; then
	printf "Installing LetsEncrypt Certificate"
	/usr/bin/certbot --nginx --nginx-server-root=/opt/gcluster/cluster-toolkit/community/front-end/ofe/website -m "${DJANGO_EMAIL}" --agree-tos -d "${SERVER_HOSTNAME}"

	printf "Installing Cron entry to keep Cert up to date"
	tmpcron=$(mktemp)
	crontab -l root >"${tmpcron}" 2>/dev/null
	echo "0 12 * * * /usr/bin/certbot renew --quiet" >>"${tmpcron}"

	# .. if something more forceful/complete is needed:
	#	echo "0 12 * * * /usr/bin/certbot certonly --force-renew --quiet" --nginx --nginx-server-root=/opt/gcluster/cluster-toolkit/community/front-end/ofe/website --cert-name "${SERVER_HOSTNAME}" -m "${DJANGO_EMAIL}" >>"${tmpcron}"

	crontab -u root "${tmpcron}"
	rm "${tmpcron}"
fi

#set +v

# eof
