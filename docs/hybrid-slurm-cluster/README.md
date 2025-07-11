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
At this moment a custom remote and branch are being used due to it being in development.

```shell
export BRANCH=hybrid
export REMOTE=jvilarru
git clone -b $BRANCH https://github.com/$REMOTE/hpc-toolkit.git /opt/cluster-toolkit && \
cd /opt/cluster-toolkit && make && make install && printf "source <(gcluster completion bash)\n" >> /root/.bashrc
```

### Copy the yaml example file
Located in community/examples/hpc-slurm6-hybrid.yaml. Copy it to a directory outside of the cluster toolkit.
For example /opt/deployments

```shell
mkdir /opt/deployments; cd /opt/deployments; cp /opt/cluster-toolkit/community/examples/hpc-slurm6-hybrid.yaml my-hybrid.yaml
```

### Copy the munge key into the deployment machine
Copy it to the deployment directory so that the image creation from packer can copy it to the cloud image.

```shell
scp slurmctld:/etc/munge/munge.key /opt/deployments/
```

### Edit the yaml file
Adapt it to your cluster. The variables on top of the file are for this:

```yaml
  local_munge_key: /opt/deployments/munge.key
  onprem_ctld_host: slurmctld
  onprem_ctld_addr: 192.168.1.40
  cluster_name: cluster
  on_prem_install_dir: /slurm/dev/24.11/inst
  slurm_uid: 620
  slurm_gid: 620
```

In this example we are stating the location of the munge key, which we have copied over in previous step. The slurm controller machine name is "slurmctld" and it has ip address 192.168.1.40. Cluster name of the slurm cluster is "cluster".
And slurm is installed in "/slurm/dev/24.11/inst", slurm uid and gid is 620.

### Copy munge key
Copy to your deployment machine, the munge key in the location that you specified in the previous step.

```shell
sudo cp /etc/munge/munge.key /opt/deployments/
sudo chown $USER: /opt/deployments/munge.key
```

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

The next steps is to remove the image preparation scripts, this is needed to avoid storing in gcp buckets the munge key.

```shell
gcluster destroy --only scripts_for_image slurm6-hybrid
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

### Correct scripts shebang
As in the cloud image a custom python is installed ensure to change all the headers of the python files in order for it to point to the python you have installed in your slurm controller machine, you can use pyenv as we did in the deployment machine to have a correct python installation.
Is important to do this step with the slurm user, as this one is the one that will be executing the scripts.
Change the variable MY_PYTHON to your python location, this also installs all the needed requirements for the scripts.

```shell
MY_PYTHON=/slurm/home/.pyenv/shims/python3
$MY_PYTHON -m pip install -r requirements.txt
find . -name "*.py" -exec sed -i "1s|^#!/slurm/python/venv/bin/python3\.13$|#!$MY_PYTHON|" {} +
```
