#!/bin/bash
# Copyright 2022 Google LLC
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

# obtain metadata of this server
GCP_PROJECT=$(curl --silent --show-error http://metadata.google.internal/computeMetadata/v1/project/project-id -H "Metadata-Flavor: Google")
SERVER_IP_ADDRESS=$(curl --silent --show-error http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip -H "Metadata-Flavor: Google")
SERVER_HOSTNAME=$(curl --silent --fail http://metadata/computeMetadata/v1/instance/attributes/hostname -H "Metadata-Flavor: Google")
config_bucket=$(curl --silent --show-error http://metadata/computeMetadata/v1/instance/attributes/webserver-config-bucket -H "Metadata-Flavor: Google")
c2_topic=$(curl --silent --show-error http://metadata/computeMetadata/v1/instance/attributes/ghpcfe-c2-topic -H "Metadata-Flavor: Google")
deploy_mode=$(curl --silent --show-error http://metadata/computeMetadata/v1/instance/attributes/deploy_mode -H "Metadata-Flavor: Google")

# Exit if deployment already exists to stop startup script running on reboots
if [[ -d /opt/gcluster/hpc-toolkit ]]
then
        printf "It appears gcluster has already been deployed. Exiting...\n"
        exit 0;
fi

printf "####################\n#### Installing required packages\n####################\n"
dnf install -y epel-release
dnf update -y --security
dnf config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo
dnf install --best -y google-cloud-sdk nano make gcc python38-devel unzip git \
    rsync nginx bind-utils policycoreutils-python-utils \
    terraform packer supervisor python3-certbot-nginx
curl --silent --show-error --location https://github.com/mikefarah/yq/releases/download/v4.13.4/yq_linux_amd64 --output /usr/local/bin/yq
chmod +x /usr/local/bin/yq
curl --silent --show-error --location https://github.com/koalaman/shellcheck/releases/download/stable/shellcheck-stable.linux.x86_64.tar.xz --output /tmp/shellcheck.tar.xz
tar xfa /tmp/shellcheck.tar.xz --strip=1 --directory /usr/local/bin

# donwload configuration file
gsutil cp "gs://${config_bucket}/webserver/config" /tmp/config
gsutil rm "gs://${config_bucket}/webserver/config"

# load configurations
DJANGO_USERNAME=$(/usr/local/bin/yq e '.django_username' /tmp/config)
DJANGO_PASSWORD=$(/usr/local/bin/yq e '.django_password' /tmp/config)
DJANGO_EMAIL=$(/usr/local/bin/yq e '.django_email' /tmp/config)
GOOGLE_CLIENT_ID=$(/usr/local/bin/yq e '.google_client_id' /tmp/config)
GOOGLE_CLIENT_SECRET=$(/usr/local/bin/yq e '.google_client_secret' /tmp/config)
repo_fork=$(/usr/local/bin/yq e '.git_fork' /tmp/config)
repo_branch=$(/usr/local/bin/yq e '.git_branch' /tmp/config)
# 'yq' does not handle multi-line string properly, need to restore the correct key format
TMP=$(/usr/local/bin/yq e '.deploy_key1' /tmp/config)
TMP2=$(echo "$TMP" | sed 's/ /\n/g')
DEPLOY_KEY1=$(echo "$TMP2" | sed -z 's/\nOPENSSH\nPRIVATE\nKEY/ OPENSSH PRIVATE KEY/g')
rm -f /tmp/config

#install go from golang as repo version is too low for hpc-toolkit
curl --silent --show-error --location https://golang.org/dl/go1.17.3.linux-amd64.tar.gz --output /tmp/go1.17.3.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go1.17.3.linux-amd64.tar.gz

#Add path entry for Go binaries to bashrc for all users (only works on future logins)
echo 'export PATH=$PATH:/usr/local/go/bin:~/go/bin' >> /etc/bashrc

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

printf "\n####################\n#### Create gcluster user & create deployment key\n####################\n"
printf "Adding gcluster user...\n"
useradd -r -m -d /opt/gcluster gcluster

if [ "${deploy_mode}" == "git" ];
then
    printf "Adding deployment keys..\n"
    mkdir -p /opt/gcluster/.ssh

    echo "$DEPLOY_KEY1" > /opt/gcluster/.ssh/gcluster-deploykey
    sed -i -e :a -e '/^\n*$/{$d;N;ba' -e '}' /opt/gcluster/.ssh/gcluster-deploykey
    cat >> /opt/gcluster/.ssh/config <<+

host github.com
        hostname github.com
        IdentityFile ~/.ssh/gcluster-deploykey
        StrictHostKeyChecking=accept-new
+
    chmod 700 /opt/gcluster/.ssh
    chmod 600 /opt/gcluster/.ssh/*
    chown gcluster -R /opt/gcluster/.ssh

    fetch_hpc_toolkit="git clone -b \"${repo_branch}\" git@github.com:${repo_fork}/hpc-toolkit.git"

elif [ "${deploy_mode}" == "tarball" ];
then
    printf "\n####################\n#### Download web application files\n####################\n"
    gsutil cp "gs://${config_bucket}/webserver/deployment.tar.gz" /tmp/deployment.tar.gz

    fetch_hpc_toolkit="tar xfz /tmp/deployment.tar.gz"
fi


#Clean up anything we may have missed
chown gcluster:gcluster -R /opt/gcluster

# run the following as 'gcluster' user
sudo su - gcluster -c /bin/bash <<EOF
  cd /opt/gcluster
  ${fetch_hpc_toolkit}

  cd /opt/gcluster/hpc-toolkit/frontend

  printf "\nDownloading Frontend dependencies...\n"
  mkdir dependencies
  pushd dependencies
  git clone -b v0.17.1 --depth 1 https://github.com/spack/spack.git
  printf "\npre-generating Spack package list\n"
  ./spack/bin/spack list > /dev/null
  popd

  printf "\nDownloading ghpc dependencies...\n"
  go install github.com/terraform-docs/terraform-docs@v0.16.0
  go install github.com/google/addlicense@latest
  pushd /opt/gcluster/hpc-toolkit
  make
  popd

  printf "\nEstablishing django environment..."
  python3.8 -m venv /opt/gcluster/django-env
  source /opt/gcluster/django-env/bin/activate
  printf "\nUpgrading pip...\n"
  pip install --upgrade pip
  printf "\nInstalling pip requirements...\n"
  pip install -r /opt/gcluster/hpc-toolkit/frontend/requirements.txt

  printf "Generating configuration file for backend..."
  echo "config:" > configuration.yaml
  echo "  server:" >> configuration.yaml
  echo "    domain_name: \"${SERVER_HOSTNAME:-${SERVER_IP_ADDRESS}}\"" >> configuration.yaml
  echo "    host_type: \"GCP\"" >> configuration.yaml
  echo "    gcp_project: \"$GCP_PROJECT\"" >> configuration.yaml
  echo "    gcs_bucket: \"${config_bucket}\"" >> configuration.yaml
  echo "    c2_topic: \"${c2_topic}\"" >> configuration.yaml

  printf "\nInitalising Django environments...\n"
  mkdir /opt/gcluster/run
  pushd website
  python manage.py makemigrations
  python manage.py migrate
  printf "\nCreating django super user..."
  DJANGO_SUPERUSER_PASSWORD=$DJANGO_PASSWORD python manage.py createsuperuser --username $DJANGO_USERNAME --email $DJANGO_EMAIL --noinput
  printf "\nInitialise Django db"
  python manage.py custom_startup_command $GOOGLE_CLIENT_ID $GOOGLE_CLIENT_SECRET
  printf "\nSet up static contents..."
  python manage.py collectstatic
  python manage.py seed_workbench_presets
  popd

  printf "\nUpdating Django settings.py...\n"
  sed -e "s/SERVER_IP/$SERVER_IP_ADDRESS/g" -e "s/SERVER_NAME/$SERVER_HOSTNAME/g" -i website/website/settings.py
  if [ -n "${SERVER_HOSTNAME}" ] ; then
    sed "s/SERVER_NAME/$SERVER_HOSTNAME/g" -i website/nginx.conf
  else
    # No server name set, remove the entry
    sed "/SERVER_NAME/d" -i website/nginx.conf
  fi

EOF

printf "Creating supervisord service..."
echo "[program:gcluster-uvicorn-background]
process_name=%(program_name)s_%(process_num)02d
directory=/opt/gcluster/hpc-toolkit/frontend/website
command=/opt/gcluster/django-env/bin/uvicorn website.asgi:application --reload --host 127.0.0.1 --port 8001
autostart=true
autorestart=true
user=gcluster
redirect_stderr=true
stdout_logfile=/opt/gcluster/run/supvisor.log" > /etc/supervisord.d/gcluster.ini

printf "Creating systemd service..."
echo "[Unit]
Description=GCluster: The GCP HPC Cluster deployment tool
Requires=supervisord.service


[Service]
Type=forking
ExecStart=/usr/sbin/nginx -p /opt/gcluster/run/ -c /opt/gcluster/hpc-toolkit/frontend/website/nginx.conf
ExecStop=/usr/sbin/nginx -p /opt/gcluster/run/ -c /opt/gcluster/hpc-toolkig/frontend/website/nginx.conf -s stop
PIDFile=/opt/gcluster/run/nginx.pid
Restart=no


[Install]
WantedBy=default.target" > /etc/systemd/system/gcluster.service

printf "Reloading systemd and starting service..."
setenforce 0
systemctl daemon-reload
systemctl start gcluster.service
systemctl status gcluster.service


# IF we have a hostname, configure for TLS
if [ -n "${SERVER_HOSTNAME}" ] ; then
    printf "Installing LetsEncrypt Certificate"
    /usr/bin/certbot --nginx --nginx-server-root=/opt/gcluster/hpc-toolkit/frontend/website -m ${DJANGO_EMAIL} --agree-tos -d ${SERVER_HOSTNAME}

    printf "Installing Cron entry to keep Cert up to date"
    tmpcron=$(mktemp)
    crontab -l root > "${tmpcron}" 2>/dev/null
    echo "0 12 * * * /usr/bin/certbot renew --quiet" >> "${tmpcron}"
    crontab -u root "${tmpcron}"
    rm "${tmpcron}"
fi

