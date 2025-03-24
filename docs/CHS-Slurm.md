## Cluster Health Scanner(CHS) on Slurm Clusters

Cluster Toolkit encourages the use of various cluster health diagnostics on Slurm clusters through the use of the following methods:

### Automatic Prolog/Epilog GPU Health Checks

Cluster Toolkit provides a built-in GPU health check script that can be automatically executed as Slurm prolog and/or epilog scripts. This script leverages nvidia-smi and dcgmi to assess the health of NVIDIA GPUs, specifically targeting Google Cloud machine types with H100, H200, B100, or B200 GPU models.

#### Functionality of Prolog/Epilog GPU Health Checks

If the health check detects failures (DCGM test failures, ECC errors, or NVLink errors), the compute node is marked as drained, preventing further job scheduling. It runs the following checks:

- DCGM Health Check: It utilizes dcgmi to run diagnostic tests and verify the overall health of the GPUs.
- ECC Error Check: It monitors and reports uncorrected volatile ECC errors using nvidia-smi.
- NVLink Error Check: It checks for NVLink errors using nvidia-smi nvlink.

Detailed logs of the health check results are written to `/var/log/slurm/chs_health_check.log` on the compute node where a job is allocated. Additionally, these logs can be accessed through the Google Cloud Logging console via the use of the below filters:

```text
SEARCH("`/var/log/slurm/chs_health_check.log`")
resource.labels.instance_id="<instance_id>"
```

where `<instance_id>` is the instance ID of the compute node, acquired from the google cloud console.

#### Configuration

The Prolog/Epilog GPU health check can be enabled or disabled through the `enable_chs_gpu_health_check_prolog` and `enable_chs_gpu_health_check_epilog` settings within the Slurm controller module.

- `enable_chs_gpu_health_check_prolog`: Enables the health check to run as a prolog script before each job step. This field is set to `true` by default.
- `enable_chs_gpu_health_check_epilog`: Enables the health check to run as an epilog script after each job step. This field is set to `false` by default.

#### Example Configuration

To enable both prolog and epilog GPU health checks, include the following settings in your Slurm controller module configuration:

```yaml
  - id: slurm_controller
    source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
    use:
    - slurm_login
    # ...
    settings:
      enable_chs_gpu_health_check_prolog: true
      enable_chs_gpu_health_check_epilog: true
      # the rest of the settings, e.g. mahcine_type, controller_startup_script, login_startup_script, etc.
```

### On-demand Cluster Health Checks

To run additional cluster health diagnostics independent of the Prolog/Epilog GPU Health Checks configured through the Toolkit, the use of [Cluster Health Scanner](https://github.com/GoogleCloudPlatform/cluster-health-scanner) is recommended. It is a tool that provides support for running DCGM Diagnostics and NCCL all_reduce_perf bus bandwidth test on A3, A3+, and A3U GPU Clusters.

To run CHS on your Slurm cluster, follow the directions [here](https://github.com/GoogleCloudPlatform/cluster-health-scanner?tab=readme-ov-file#2-running-via-cluster_diag) to clone the CHS repo and install the required dependencies. Running health checks on Slurm can be done either through CHS's [`healthscan`](https://github.com/GoogleCloudPlatform/cluster-health-scanner/blob/main/cli/healthscan.py) CLI or directly through the [`cluster-validation.sh`](https://github.com/GoogleCloudPlatform/cluster-health-scanner/blob/main/deploy/slurm/cluster-validation.sh) bash script.
