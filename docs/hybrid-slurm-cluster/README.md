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

### Create the bucket and upload the slurm key
The compute nodes do not have the key baked into their image. On boot they mount a bucket with gcsfuse, copy the key locally and unmount it again, so the key has to be in the bucket before the nodes start.
This is your own bucket, it is not the one the toolkit creates for the cluster files, so you can apply whatever restrictions you need to it. The deployment does not create it, it has to exist beforehand.

The example uses slurm authentication ("enable_slurm_auth: true"), which is the recommended option, so the file needed is the "slurm.key" your on-prem controller authenticates with, normally in the slurm config directory.
Munge is supported as well, if that is what your on-prem cluster uses set "enable_slurm_auth: false" in the yaml, replace "slurm_key_mount" with "munge_mount" and upload "munge.key" instead. The rest of the steps are the same either way, what matters is that the key matches the authentication your controller already runs.

```shell
gcloud storage buckets create gs://your_pre_existing_bucket --location=us-central1
gcloud storage cp /slurm/dev/24.11/inst/etc/slurm.key gs://your_pre_existing_bucket/keys/slurm.key
```

The compute nodes need to be able to read that object, and as this bucket is not managed by the deployment you have to grant it yourself.
Unless you set a service account on the nodeset, the compute nodes use the default compute service account:

```shell
PROJECT_NUMBER=$(gcloud projects describe your_project_id --format='value(projectNumber)')
gcloud storage buckets add-iam-policy-binding gs://your_pre_existing_bucket \
    --member=serviceAccount:$PROJECT_NUMBER-compute@developer.gserviceaccount.com \
    --role=roles/storage.objectViewer
```

If the compute nodes cannot read the key they will fail to mount it at boot, so it is worth checking this before deploying.

### Edit the yaml file
Adapt it to your cluster. The variables on top of the file are for this:

```yaml
  key_bucket: your_pre_existing_bucket
  key_prefix: keys
  onprem_ctld_host: slurmctld
  onprem_ctld_addr: 192.168.1.40
  cluster_name: cluster
  on_prem_install_dir: /slurm/dev/24.11/inst
  slurm_uid: 620
  slurm_gid: 620
```

The slurm controller machine name is "slurmctld" and it has ip address 192.168.1.40. Cluster name of the slurm cluster is "cluster".
And slurm is installed in "/slurm/dev/24.11/inst", slurm uid and gid is 620.

"key_bucket" is the bucket you created in the previous step and "key_prefix" the directory inside it where the key was uploaded. It is only used to fetch the key, the cluster files still go to the bucket the deployment creates.

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
cd /opt/deployments/output
./install_hybrid.sh
```

The conf.tgz file gets generated in the output dir, this contains all the needed files for the slurm controller.
## Slurm controller machine
These last steps need to be done on the slurm controller machine.
### Copy the conf.tgz
Copy the tar file to the slurm controller machine.
### Merge the config files
Merge the config files generated in the tar file and copy also the scripts file to your slurm configuration directory, in the example this directory is /slurm/dev/24.11/etc

```shell
mkdir /slurm/dev/24.11/etc/cloud_files; cd /slurm/dev/24.11/etc/cloud_files
tar xf /tmp/config.tgz
#Move all scripts to etc, it also includes hidden files
bash -c 'shopt -s dotglob; mv scripts/* ../'
#Move all the cloud config files to etc, my slurm.conf includes cloud.conf, the gres.conf includes cloud_gres.conf etc… In the not-example case, compare your slurm.conf with the generated one.
mv cloud*.conf ../
```

This example is for an ideal situation in which the slurm.conf, gres.conf etc... are already prepared to include all the cloud config files, in each case ensure to merge your slurm.conf with the autogenerated one, also ensure that your gres.conf has a include cloud_gres.conf, the same for topology.conf
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
