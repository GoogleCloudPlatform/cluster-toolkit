# Hybrid Slurm Clusters

NOTE: This guide assumes that all the time you are using root user for all the operations unless specified.

Different steps will be done in different machines, for simplicity we will be referring to these machines as the "deployment machine" and "slurm machine". These machines can be the same one, but this guide has been written in a way that the slurm controller machine can run as few things as possible.

We will start with the deployment machine. This can be any machine, even a docker container, the only requirement is that it can access the gcp cloud infrastructure using gcloud.

## Deployment machine
### Install system packages
Use your OS package manager.
For debian based:

```shell
apt install -y python3 curl python3-pip git make golang unzip bash-completion zip libbz2-dev liblzma-dev libsqlite3-dev libncurses-dev libreadline-dev libffi-dev libssl-dev zlib1g-dev
```

RHEL based:

```shell
dnf install -y python3 curl python3-pip git make golang unzip bash-completion zip bzip2 xz sqlite-devel ncurses-devel readline-devel libffi-devel openssl-devel zlib-devel
```

### Install gcloud

```shell
curl -o /tmp/gcloud.tar.gz https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-linux-x86_64.tar.gz && \
tar xf /tmp/gcloud.tar.gz -C /opt && \
rm /tmp/gcloud.tar.gz && \
/opt/google-cloud-sdk/install.sh -q --path-update true --command-completion true

#configuration, this will ask for you to go to a gcp page on your browser and enter a code here.
gcloud init --no-launch-browser --skip-diagnostics
gcloud auth application-default login --no-launch-browser
```

### Install terraform
https://learn.hashicorp.com/tutorials/terraform/install-cli

```shell
export TERRAFORM_VERSION=1.12.0
curl -fsSL https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip -o terraform.zip && \
   unzip terraform.zip && \
   mv terraform /usr/local/bin/ && \
   rm terraform.zip
```

### Install packer
https://learn.hashicorp.com/tutorials/packer/get-started-install-cli

```shell
export PACKER_VERSION=1.12.0
curl -fsSL https://releases.hashicorp.com/packer/${PACKER_VERSION}/packer_${PACKER_VERSION}_linux_amd64.zip -o packer.zip && \
   unzip packer.zip && \
   mv packer /usr/local/bin/ && \
   rm packer.zip
```

### Install pyenv
We will need this to ensure that we have a working python and all the needed dependencies.

```shell
curl -fsSL https://pyenv.run | bash
echo 'export PYENV_ROOT="/root/.pyenv"' >> /root/.bashrc && \
echo '[[ -d $PYENV_ROOT/bin ]] && export PATH="$PYENV_ROOT/bin:$PATH"' >> /root/.bashrc && \
echo 'eval "$(pyenv init - bash)"' >> /root/.bashrc && \
echo 'eval "$(pyenv virtualenv-init -)"' >> /root/.bashrc
/root/.pyenv/bin/pyenv install 3.10
/root/.pyenv/bin/pyenv global 3.10
/root/.pyenv/shims/python -m ensurepip && /root/.pyenv/shims/python -m pip install --upgrade pip
```

### Install cluster-toolkit

```shell
git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git /opt/cluster-toolkit && \
cd /opt/cluster-toolkit && make && make install && printf "source <(gcluster completion bash)\n" >> /root/.bashrc
```

### Copy the yaml example file
Located in community/examples/hpc-slurm6-hybrid.yaml. Copy it to a directory outside of the cluster toolkit.
For example /opt/deployments

```shell
mkdir /opt/deployments; cd /opt/deployments; cp /opt/cluster-toolkit/community/examples/hpc-slurm6-hybrid.yaml my-hybrid.yaml
```

### Export the slurm key from the controller
The compute nodes do not have the key baked into their image. On boot they mount a directory the on-prem controller exports over NFS, copy the key locally and unmount it again, so the key is never written into a disk image and never leaves your controller except for that mount.

The example uses slurm authentication ("enable_slurm_auth: true"), which is the recommended option, so the file needed is the "slurm.key" your on-prem controller authenticates with, normally in the slurm config directory.
Munge is supported as well, if that is what your on-prem cluster uses set "enable_slurm_auth: false" in the yaml and replace "slurm_key_mount" with "munge_mount" pointing at a directory holding "munge.key". The rest of the steps are the same either way, what matters is that the key matches the authentication your controller already runs.

On the slurm controller, put the key in the directory you set in "key_export_dir" and export it to the cloud subnet only:

```shell
mkdir -p /slurm/key_distribution
cp /slurm/dev/<slurm_version>/inst/etc/slurm.key /slurm/key_distribution/
chown -R slurm:slurm /slurm/key_distribution
chmod 700 /slurm/key_distribution; chmod 400 /slurm/key_distribution/slurm.key
echo "/slurm/key_distribution 10.0.0.0/24(ro,no_subtree_check,no_root_squash)" >> /etc/exports
exportfs -ra
```

Restrict the export to the subnet the cloud nodes come from, 10.0.0.0/24 in this example, and make sure the VPN lets NFS through from that subnet to the controller.
The nodes mount it as root and copy the key to 0400 owned by the slurm user, so a job running on a cloud node cannot read it afterwards.

If the compute nodes cannot reach the export they will fail to mount it at boot, so it is worth checking this before deploying.

### Edit the yaml file
Adapt it to your cluster. The variables on top of the file are for this:

```yaml
  key_export_dir: /slurm/key_distribution
  onprem_ctld_host: slurmctld
  onprem_ctld_addr: 192.168.1.40
  cluster_name: cluster
  on_prem_install_dir: /slurm/dev/<slurm_version>/inst
  slurm_uid: 620
  slurm_gid: 620
```

The slurm controller machine name is "slurmctld" and it has ip address 192.168.1.40. Cluster name of the slurm cluster is "cluster".
And slurm is installed in "/slurm/dev/<slurm_version>/inst", slurm uid and gid is 620.
The paths in this guide use "<slurm_version>" as a placeholder, replace it with the version your on-prem cluster runs. Keep "slurm_version" in the yaml on that same version, it is the one the compute image gets built with, and the cloud nodes have to match the controller.

"key_export_dir" is the directory you exported in the previous step, the compute nodes mount it from the controller to get the key.

### Credentials for the on-prem controller
The on-prem controller runs "resume.py" and "suspend.py" to create and delete the cloud nodes, so it needs its own GCP credentials. Create a service account, give it "roles/compute.instanceAdmin.v1" and "roles/iam.serviceAccountUser" on the project, plus "roles/storage.objectViewer" on the bucket holding the cluster config, download a key for it and point "hybrid_conf.google_app_cred_path" at the file on the controller:

```yaml
      hybrid_conf:
        google_app_cred_path: /slurm/dev/<slurm_version>/inst/etc/cloud_sa.json
```

Without it the resume and suspend programs cannot talk to the compute API and the cloud nodes never come up.

### Name resolution between on-prem and the cloud
Cloud DNS is turned off for hybrid, and the generated config does not set NodeAddr for the cloud nodes, so your on-prem slurmctld has to be able to resolve their names. Set up DNS forwarding to the GCP internal zone over the VPN, or your controller will not reach the nodes it just resumed.
In the other direction the cloud nodes have to reach the controller: set "onprem_ctld_addr" as this example does, and the name does not need to resolve from the cloud.

### Create the deployment

```shell
gcluster create my-hybrid.yaml
```

### Deploy cloud resources
This will create the network, the scripts needed to customize the image and the instances templates that the compute nodes will be spawned from.
It will also generate some files in the output dir that we will be using later.

```shell
gcluster deploy slurm6-hybrid
```

In case that you need to redeploy the cluster be sure to only do the cluster, as if not you will be generating a new packer image, this can be done like this:

```shell
gcluster deploy --only cluster slurm6-hybrid
```

### [VPN setup](./vpn.md)
In this document an example on how to setup a site vpn between your network and the google network we just created is shown, check with your network administrator if that is the desired way to proceed.
Normally all the enterprise routers have a ipsec vpn solution, which is what is being explained in the document.

### Create the configurations
Execute the install_hybrid script, this will generate a tar file that we will need in the next step.

```shell
pyenv shell 3.10
cd /share/slurm6-hybrid-files
./install_hybrid.sh
```

The config.tgz file gets generated in the output dir, the one you set in "output_dir", and contains all the needed files for the slurm controller.
## Slurm controller machine
These last steps need to be done on the slurm controller machine.
### Copy the config.tgz
Copy the tar file to the slurm controller machine.
### Merge the config files
Merge the config files generated in the tar file and copy also the scripts file to your slurm configuration directory, in the example this directory is /slurm/dev/<slurm_version>/etc

```shell
mkdir /slurm/dev/<slurm_version>/etc/cloud_files; cd /slurm/dev/<slurm_version>/etc/cloud_files
tar xf /tmp/config.tgz
#Move all scripts to etc, it also includes hidden files
bash -c 'shopt -s dotglob; mv scripts/* ../'
#Move all the cloud config files to etc, my slurm.conf includes cloud.conf, the gres.conf includes cloud_gres.conf etc… In the not-example case, compare your slurm.conf with the generated one.
#From 25.x the topology lives in cloud_topology.yaml, so the yaml files go too.
mv cloud*.conf cloud*.yaml ../
#The scripts come out of the tarball owned by whoever ran install_hybrid.sh and
#with no permissions for anyone else, slurmctld runs them as SlurmUser.
chown -R slurm:slurm ../
```

This example is for an ideal situation in which the slurm.conf, gres.conf etc... are already prepared to include all the cloud config files, in each case ensure to merge your slurm.conf with the autogenerated one, also ensure that your gres.conf has a include cloud_gres.conf, and that your topology file includes the generated one, cloud_topology.yaml from 25.x on
Also be sure to include the ["SuspendExcParts"](https://slurm.schedmd.com/slurm.conf.html#OPT_SuspendExcParts) configuration option in order to exclude your on-prem partitions from the powersaving mechanism.
It is also recommended to set ["CommunicationParameters=NoAddrCache"](https://slurm.schedmd.com/slurm.conf.html#OPT_NoAddrCache), as cloud nodes get a new IP address every time they are recreated and slurmctld would otherwise keep trying to reach the address they had before.

### Correct scripts shebang
As in the cloud image a custom python is installed ensure to change all the headers of the python files in order for it to point to the python you have installed in your slurm controller machine, you can use pyenv as we did in the deployment machine to have a correct python installation.
Is important to do this step with the slurm user, as this one is the one that will be executing the scripts.
Change the variable MY_PYTHON to your python location, this also installs all the needed requirements for the scripts.

```shell
MY_PYTHON=/slurm/home/.pyenv/shims/python3
$MY_PYTHON -m pip install -r requirements.txt
find . -name "*.py" -exec sed -i "1s|^#!/slurm/python/venv/bin/python3\.13$|#!$MY_PYTHON|" {} +
```
