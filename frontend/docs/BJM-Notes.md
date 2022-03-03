# Notes

  - OSC OpenOnDemand - doesn't really seem like the right fit.  More focused on web-app front-end to clusters.

  - CitC supports Filestore/EFS (I think...) 
  - CitC supports multiple instance types in the "cluster" via SLURM
    - `limits.yaml` sets your acceptable quota - need to investigate how this corresponds to SLURM Partitions (or some other mechanism to pick instance types at submission time)
      - `-C FEATURE` to srun can be used to limit to nodes with particular 'features' set.  (ie: `--constraint="intel"`) [Link](https://slurm.schedmd.com/srun.html#OPT_constraint)
      - `./ansible/roles/slurm/files/update_config.py` sets features:
        - `shape={shape}` (shape-name from `limits.yaml`)
        - `ad={ad}`
        - `arch={arch}`  (x86_64, aarch64)
      - **SUFFICIENT TO SUBMIT JOBS WITH -C shape=\<shape>**


- CitC
  - config steps
    - login update - centos@<ip> -> Update ~citc/.ssh/authorized_keys
    - set limits.yaml
    - install spack into `/mnt/shared/spack`
    - update `compute_image_extra.sh` to make sourcing spack part of login (`/etc/profile.d/spack` ?)
    - `sudo /usr/local/bin/run-packer`
    - `sudo /usr/local/bin/run-packer aarch64` (on AWS for graviton2)
- 



# DEMO FOR AWS

~~~bash
$ terraform apply -auto-approve aws
#... <~3 minute wait>
ManagementPublicIP = 3.129.60.142
cluster_id = smooth-termite
$ ssh -i ~/.ssh/aws-key centos@3.129.60.142
The authenticity of host '3.129.60.142 (3.129.60.142)' can't be established.
ECDSA key fingerprint is SHA256:qqZ8PvlmJmCkzgq2FjxUsdbcbLXAsZnE2mbUEh5jwxE.
Are you sure you want to continue connecting (yes/no)? yes

[centos@mgmt ~]$ sudo su -l citc
[citc@mgmt ~]$ sudo tail -f /root/ansible-pull.log
#... <~18 minute wait for ansible/packer build>
[citc@mgmt ~]$ cat > limits.yaml <<+
> t3a.medium: 4
> t4g.2xlarge: 4
> +
> # Above needs to be chosen by user somehow

[citc@mgmt ~]$ finish
[citc@mgmt ~]$ mkdir /mnt/shared/spack
[citc@mgmt ~]$ sudo git clone --depth 1 --branch v0.16.0 https://github.com/spack/spack.git /mnt/shared/spack
[citc@mgmt ~]$ chown -R citc:citc /mnt/shared/spack
[citc@mgmt ~]$ cat >> compute_image_extra.sh <<EOF
sudo yum -y groupinstall "Development Tools"
sudo yum -y install cmake gcc-gfortran

# EFA - brings in OpenMPI
curl -O https://efa-installer.amazonaws.com/aws-efa-installer-1.11.1.tar.gz
tar -xf aws-efa-installer-1.11.1.tar.gz
cd aws-efa-installer
arch=$(uname -m)
if [ "$arch" = "aarch64" ]; then
    # We want OpenMPI, same as on x86 - but EFA on CentOS on ARM isn't yet supported.
    sudo ./efa_installer.sh -y -k
else
    sudo ./efa_installer.sh -y
fi

echo 'kernel.yama.ptrace_scope = 0' | sudo tee /etc/sysctl.d/10-ptrace.conf


# Make sure that spack has found this arch's compiler & apps
sudo /mnt/shared/spack/bin/spack compiler find --scope=system
sudo -E PATH=/opt/amazon/openmpi/bin:$PATH /mnt/shared/spack/bin/spack external find --scope system --not-buildable

# Mark GCC & cmake as buildable, as some may want newer GCC
sudo /mnt/shared/spack/bin/spack config --scope system add packages:gcc:buildable:true
sudo /mnt/shared/spack/bin/spack config --scope system add packages:cmake:buildable:true

echo '. /mnt/shared/spack/share/spack/setup-env.sh' | sudo tee /etc/profile.d/99-spack.sh

EOF
# Consider increasing size of Image builders:
# Takes a LONG time (hung? stuck?) with default t2.nano to install 'efa' package (perhaps DKMS in background?)
#
# /usr/local/bin/run-packer
#  'aws_instance_type="t3a.medium"'  or t4g.medium for ARM
[citc@mgmt ~]$ sudo /usr/local/bin/run-packer
[citc@mgmt ~]$ sudo /usr/local/bin/run-packer aarch64

~~~

####EFA + ARM:
**An EFA kernel driver on this operating system for the Arm architecture is not available in this version of the installer.**
Supported AMIs for EFA on Graviton:  Amazon Linux 2, Ubuntu 18.04/20.04, OpenSUSE Leap 15.2

Can pass '-k' to efa_installer.sh on AARCH64

Need to "find" the compiler on ARM, too:
`spack compiler find --scope=site`


Test:  `spack install hpl`

### ARM + BLAS
[Download ArmPL for blas/lapack](https://developer.arm.com/tools-and-software/server-and-hpc/downloads/arm-performance-libraries)


# Placement Groups
Placement groups are a little tricky.  Ideally, we'd have a placement group per instance type, but we don't know what all instance types a cluster might have at Terraform time.  If we want users from the web interface to pre-define instance types at cluster creation time, (with no ability to change it later) then we could modify the terraform template to add a placement group per instance type, as well as pre-fill the `~citc/limits.yaml` file.

Barring that, we can have Terraform just create a single placement group, and launch all compute instances within that placement group.  There is just a higher likelyhood of launch failures occurring in that case.  Failures could easily occur due to:

  * General Cloud Capacity issues (placement doesn't really change it, but makes it more likely, as it reduces flexibility)
  * Heterogeneous node types can cause increased likelyhood of placement

Placement groups are nicely associated with a "job" - however, SLURM boots/removes nodes "at will", and will re-use nodes and mix&match nodes to jobs over their lifetime (potentially). This makes it incorrect to rely on `startnode` and `stopnode` calls to create/remove placement groups.

### AWS

Placement Groups can be created, and instances added / removed at will.  No pre-set size.  Incompatible with "burst VMs" (T-series) and Mac1.

### GCP
Called the "group-placement" "resource_policy" on Google:

```sh
gcloud compute resource-policies create group-placement compact-placement  --collocation COLLOCATED --vm-count 10 --project gcluster-discovery
```

Must have quota for 'AFFINITY_GROUPS':

```
ERROR: (gcloud.compute.resource-policies.create.group-placement) Could not fetch resource:
 - Quota 'AFFINITY_GROUPS' exceeded.  Limit: 0.0 in region us-central1.
```

Documentation for [this](https://cloud.google.com/compute/docs/instances/define-instance-placement#compact) says:
> VM_COUNT: the number of VM instances to include in that policy. For compact policies, you **must** apply the policy to exactly this number of instances.

also

> Compact:
> 
>   * **Up to 22 instances** in each policy
>   * Applies to a fixed number of instances
>   * Support **only for C2** machine types

(emphasis is mine).


### Implementation conclusions from above

1. Don't try to have Terraform create the placement groups, unless at TF time, we know the max size of the cluster.
  - Could make this part of the `finish` call (`/usr/local/bin/update_config`) to create placement groups based off `limits.yaml`
  - Would need to manage cleanup of these, somehow. (delete all that match a name (`${cluster_id}-placement-${shape}`), create new ones?)
1. Create placement groups on a per-instance-type basis
1. Make the `start_node` method become a `start_nodes` method, and support starting multiple instances with a single invocation of `startnode` from SLURM (it can pass a list of nodes)
1. `start_nodes` applies the resource policy/placement group if appropriate.
