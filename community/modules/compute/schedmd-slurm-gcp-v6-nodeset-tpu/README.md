## Description

This module creates partition of [TPU](https://cloud.google.com/tpu/docs/intro-to-tpu) nodeset.
TPUs are Google's custom-developed application specific ICs to accelerate machine
learning workloads.

TPU nodes run on one of the predefined TPU node runtimes and runs Slurm from within Docker container.

To set runtime version, please consult [runtimes documentation](https://cloud.google.com/tpu/docs/runtimes) and set appropriate runtime for your use case and TPU version. There
are prebuilt docker containers including different TensorFlow versions included available at `us-docker.pkg.dev/schedmd-slurm-public/tpu/slurm-gcp-6-9:tf-<tensorflow version in x.y format>`.

### Example

The following code snippet creates TPU partition with following attributes.

- TPU nodeset module is connected to `network` module.
- TPU nodeset is of type `v2-8`, runtime_version `2.10.0`, and uses `us-docker.pkg.dev/schedmd-slurm-public/tpu/slurm-gcp-6-9:tf-2.10` docker image
- TPU vms are preemptible.
- `preserve_tpu` is set to false. This means, suspended vms will be deleted.
- Partition module uses this defined `tpu_nodeset` module and this partition can
be accessed as `tpu` partition.

```yaml
  - id: tpu_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset-tpu
    use: [network]
    settings:
      node_type: v2-8
      tf_version: 2.10.0
      runtime_version: tpu-vm-tf-2.10
      docker_image: us-docker.pkg.dev/schedmd-slurm-public/tpu/slurm-gcp-6-9:tf-2.10 
      disable_public_ips: false
      preemptible: true
      preserve_tpu: false

  - id: tpu_partition
    source: community/modules/compute/schedmd-slurm-gcp-v6-partition
    use: [tpu_nodeset]
    settings:
      partition_name: tpu
```

### Running MPI workloads on TPU nodes
Because jobs are running in Docker container, MPI is getting confused about which interface can be used to communicate
between the nodes. Set following environment variable, to make sure that correct interface is used for communication:

```bash
export OMPI_MCA_btl_tcp_if_exclude="docker0,127.0.0.0/8"
```

### Building your own Docker container
Docker container base operating system should match image used for controller and login instances to prevent any issues
with running shared binaries.

To run Ansible playbooks following packages has to be installed first:
- ansible
- curl
- git
- google-cloud-cli
- python3-libselinux
- python3-pip
- selinux-policy
- systemd

To build a container image adapt following Dockerfile to your needs:

```Dockerfile
FROM rockylinux/rockylinux:9.4

RUN dnf install -y python3-pip git python3-libselinux selinux-policy systemd && \
    ln -s /usr/lib/systemd/systemd /usr/bin/systemd && \
    dnf install -y --allowerasing curl && \
    touch /etc/fstab && \
    chmod 755 /etc && \
    ( \
        echo "[google-cloud-cli]" ; \
        echo "name=Google Cloud CLI" ; \
        echo "baseurl=https://packages.cloud.google.com/yum/repos/cloud-sdk-el9-x86_64" ; \
        echo "enabled=1" ; \
        echo "gpgcheck=1" ; \
        echo "repo_gpgcheck=0" ; \
        echo "gpgkey=https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg" ; \
    ) | tee -a /etc/yum.repos.d/google-cloud-sdk.repo && \
    dnf install -y google-cloud-cli && \
    pip3 install ansible==6.7.0 && \
    export PATH=/usr/local/bin:$PATH && \
    export PYTHONUNBUFFERED=1 && \
    ansible --version && \
    ansible-galaxy collection install ansible.posix && \
    curl -sSO https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh && \
    bash add-google-cloud-ops-agent-repo.sh --also-install && \
    echo '{  \
      "install_libtpu": false, \
      "tf_version": "none", \
      "monitoring_agent": "cloud-ops" \
    }' > /tmp/vars.json && \
    ansible-pull \
        -U https://github.com/GoogleCloudPlatform/slurm-gcp -C 6.9.1 \
        -i localhost, --limit localhost --connection=local \
        -e @/tmp/vars.json \
        ansible/docker-playbook.yml
```

When using custom Docker image stored in Artifact Registry, grant `roles/artifactregistry.reader` role to service
account used by TPU nodes.

### Running LLM straining on TPUs
[Torchprime](https://github.com/AI-Hypercomputer/torchprime) is a reference implementation for training PyTorch models
on TPUs. it can be used to run the training jobs on the TPU nodes managed by Slurm.

On a mount point that is shared across the cluster (`/home` for example), create a Python virtual environment for TorchPrime:

```bash
PYTHON_ENV_PATH=venv

python3.11 -m venv ${PYTHON_ENV_PATH}
source ${PYTHON_ENV_PATH}/bin/activate

pip install --pre torch torchvision --index-url https://download.pytorch.org/whl/nightly/cpu
pip install 'torch_xla[tpu] @ https://storage.googleapis.com/pytorch-xla-releases/wheels/tpuvm/torch_xla-2.8.0.dev-cp311-cp311-linux_x86_64.whl' \
  -f https://storage.googleapis.com/libtpu-releases/index.html \
  -f https://storage.googleapis.com/libtpu-wheels/index.html

git clone https://github.com/AI-Hypercomputer/torchprime.git
cd torchprime
pip install -e '.[dev]'
```

And then a sbatch script `submit.sh`:

```bash
#!/bin/bash

#SBATCH --job-name=torchprime
#SBATCH --time=01:00:00
#SBATCH --mem=650G

set -o xtrace

source venv/bin/activate

export JOB_DIR=$(pwd)/job-outputs/slurm-${SLURM_JOB_ID}-${SLURM_JOB_NAME}
export PJRT_DEVICE=TPU
ulimit -n 1048576
# too low maximum locked memory limit may result in the job freezing during the training  
ulimit -l 68719476736

cd torchprime
# ICI_FSDP must match accelerator type 32 for a node with 32 TPUs
export ICI_FSDP=32
echo ici_fsdp=$ICI_FSDP
# DCN_FSDP is used for multi-slice training
export DCN_FSDP=${SLURM_NNODES}
echo dcn_fsdp=$DCN_FSDP

# all MEGASCALE_ environment variables are necessary for multi-slice training
export MEGASCALE_COORDINATOR_ADDRESS=$(hostname -i)
export MEGASCALE_NUM_SLICES=${SLURM_NNODES}

# Using bash, so the variable expansion will happen on the compute node
srun --label /bin/bash -o xtrace -c '
export XLA_FLAGS="${XLA_FLAGS} --xla_dump_to=${JOB_DIR}/xla_dumps/${SLURMD_NODENAME}/ --xla_dump_hlo_as_proto --xla_dump_hlo_as_text"
export MEGASCALE_SLICE_ID=${SLURM_NODEID}
python torchprime/torch_xla_models/train.py \
        model=llama-3.1-8b \
        profile_dir=${JOB_DIR}/profile/${SLURMD_NODENAME}/ \
        global_batch_size=64 \
        block_size=8192 \
        max_steps=30 \
        profile_step=5 \
        ici_mesh.fsdp=$ICI_FSDP \
        dcn_mesh.fsdp=$DCN_FSDP'
```

Before submitting the job you need to login into Hugging Face:

```bash
./venv/bin/huggingface-cli login
```

Then submit the job using:

```bash
sbatch -p tpu32 submit.sh
```

This assumes, that ICI_FSDP was set to `32`. If you want to run multi-slice training, just add how many number of nodes
you want to spin. For 2 slices of 32 TPU use:

```batch
sbatch -p tpu32 -N 2 submit.sh
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_accelerator_config"></a> [accelerator\_config](#input\_accelerator\_config) | Nodeset accelerator config, see https://cloud.google.com/tpu/docs/supported-tpu-configurations for details. | <pre>object({<br/>    topology = string<br/>    version  = string<br/>  })</pre> | <pre>{<br/>  "topology": "",<br/>  "version": ""<br/>}</pre> | no |
| <a name="input_data_disks"></a> [data\_disks](#input\_data\_disks) | The data disks to include in the TPU node | `list(string)` | `[]` | no |
| <a name="input_disable_public_ips"></a> [disable\_public\_ips](#input\_disable\_public\_ips) | DEPRECATED: Use `enable_public_ips` instead. | `bool` | `null` | no |
| <a name="input_docker_image"></a> [docker\_image](#input\_docker\_image) | The gcp container registry id docker image to use in the TPU vms, it defaults to us-docker.pkg.dev/schedmd-slurm-public/tpu/slurm-gcp-6-9:tf-none | `string` | `null` | no |
| <a name="input_enable_public_ips"></a> [enable\_public\_ips](#input\_enable\_public\_ips) | If set to true. The node group VMs will have a random public IP assigned to it. Ignored if access\_config is set. | `bool` | `false` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the nodeset. Automatically populated by the module id if not set. <br/>If setting manually, ensure a unique value across all nodesets. | `string` | n/a | yes |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on nodes. | <pre>list(object({<br/>    server_ip     = string,<br/>    remote_mount  = string,<br/>    local_mount   = string,<br/>    fs_type       = string,<br/>    mount_options = string,<br/>  }))</pre> | `[]` | no |
| <a name="input_node_count_dynamic_max"></a> [node\_count\_dynamic\_max](#input\_node\_count\_dynamic\_max) | Maximum number of auto-scaling worker nodes allowed in this partition. <br/>For larger TPU machines, there are multiple worker nodes required per machine (1 for every 8 cores).<br/>See https://cloud.google.com/tpu/docs/v4#large-topologies, for more information about these machine types. | `number` | `0` | no |
| <a name="input_node_count_static"></a> [node\_count\_static](#input\_node\_count\_static) | Number of worker nodes to be statically created. <br/>For larger TPU machines, there are multiple worker nodes required per machine (1 for every 8 cores).<br/>See https://cloud.google.com/tpu/docs/v4#large-topologies, for more information about these machine types. | `number` | `0` | no |
| <a name="input_node_type"></a> [node\_type](#input\_node\_type) | Specify a node type to base the vm configuration upon it. | `string` | `""` | no |
| <a name="input_preemptible"></a> [preemptible](#input\_preemptible) | Should use preemptibles to burst. | `bool` | `false` | no |
| <a name="input_preserve_tpu"></a> [preserve\_tpu](#input\_preserve\_tpu) | Specify whether TPU-vms will get preserve on suspend, if set to true, on suspend vm is stopped, on false it gets deleted | `bool` | `false` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID to create resources in. | `string` | n/a | yes |
| <a name="input_reserved"></a> [reserved](#input\_reserved) | Specify whether TPU-vms in this nodeset are created under a reservation. | `bool` | `false` | no |
| <a name="input_runtime_version"></a> [runtime\_version](#input\_runtime\_version) | Nodeset runtinme version, see https://cloud.google.com/tpu/docs/runtimes#tpu_vm for details. | `string` | `"tpu-ubuntu2204-base"` | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | DEPRECATED: Use `service_account_email` and `service_account_scopes` instead. | <pre>object({<br/>    email  = string<br/>    scopes = set(string)<br/>  })</pre> | `null` | no |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | Service account e-mail address to attach to the TPU-vm. | `string` | `null` | no |
| <a name="input_service_account_scopes"></a> [service\_account\_scopes](#input\_service\_account\_scopes) | Scopes to attach to the TPU-vm. | `set(string)` | <pre>[<br/>  "https://www.googleapis.com/auth/cloud-platform"<br/>]</pre> | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script used by VMs in this nodeset | `string` | `"# no-op"` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The name of the subnetwork to attach the TPU-vm of this nodeset to. | `string` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone in which to create compute VMs. TPU partitions can only specify a single zone. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nodeset_tpu"></a> [nodeset\_tpu](#output\_nodeset\_tpu) | Details of the nodeset tpu. Typically used as input to `schedmd-slurm-gcp-v6-partition`. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
