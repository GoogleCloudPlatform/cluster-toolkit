# Simple Ipynb Launcher
The Simple Ipynb Launcher is a Jupyter Notebook-based interface designed to run folding jobs on the Alphafold 3 High Throughput solution. We are using Google's Vertex AI Workbench services to host the Jupyter notebook and configure it to interoperate with the Slurm cluster via the Slurm REST API. For convenience, we provide a Jupyter notebook that can run folding jobs and analyze and visualize the output.

## Setup Guide: Jupyter Notebook with SLURM
Please note that the launcher needs 2 specific setup steps:

- **[Setup AF3 with Simple Ipynb Launcher - PART 1: specific cluster settings](./Setup-pre-cluster-deployment.md)**: Instructions to be followed before bringing up the `af3-slurm.yaml` Slurm cluster.

- **[Setup: Setup AF3 with Simple Ipynb Launcher - PART 2: IPython notebook setup](./Setup-post-cluster-deployment.md)**: Instructions for launching the notebook environment.

## Usage Guide
For usage of the Ipynb Launcher consult the [Step-by-Step Instructions](./Ipynb.md)

## Known Issues
You may encounter the following problems while using the notebook.

### Warning during dependency installation

**Description**:
You may encounter the following warning during dependency installation:

```text
ERROR: pip's dependency resolver does not currently take into account all the packages that are installed. This behaviour is the source of the following dependency conflicts.
google-cloud-bigtable 1.7.3 requires grpc-google-iam-v1<0.13dev,>=0.12.3, but you have grpc-google-iam-v1 0.14.2 which is incompatible
```

**Resolution**: This warning can be **safely ignored**.

The version mismatch does not impact the functionality required by this project. The `google-cloud-bigtable` package is not used in any critical code path, and no issues have been observed during execution.

### Resource Unavailability (Out of Capacity)

**Description**:  
Your job is queued but not executed because GCP can't provision the required compute nodes. This is often due to a temporary lack of available resources in the chosen region or zone.

**Resolution**:
- Check the Slurm **Controller Node** Log:  
  Log on to the controller machine for your cluster to see detailed error messages like  
  `"GCP Error: Zone does not currently have sufficient capacity for resources"`.
- **Wait and Re-check**:  
  For temporary resource shortages, wait **5–10 minutes** and then re-run the `get_job_state` cell in your Jupyter notebook. Resources might become available and allow your job to proceed.

### Node Creation In Progress

**Description**:  
After submitting your job, the cluster may take several minutes to provision and initialize new compute nodes. During this time, your job might temporarily show a status like `NODE_FAIL` if the nodes are not yet fully ready or registered.

**Resolution**:
- Check the Slurm **Controller Node** Logs for detailed status updates.
- **Wait 5–10 minutes**, then recheck your job status to see if the nodes have become available and the job is progressing.

### Input File Format Error

**Description**:  
Your job might fail immediately if the input file format is incorrect or if the file is corrupted.

**Resolution**:
- Review the **Job Logs** to identify issues related to the input file. Logs are located at:
  - `/home/af3ipynb/datapipeline_result/<input_file_name>/<timestamp>/slurm_logs/job_<job_id>/*.txt` for **data pipeline** process
  - `/home/af3ipynb/inference_result/<input_file_name>/<timestamp>/slurm_logs/job_<job_id>/*.txt` for **inference** process.
  
  Each directory contains:
  - `err.txt` – captures detailed **STDERR** output.
  - `out.txt` – captures **STDOUT** output.

- Ensure that the input file:
  - Conforms to the expected format. Currently supported formats include alphafoldserver and alphafold3.
  - Is not corrupted.

For detailed input file requirements, please refer to [Alphafold Input Documentation](https://github.com/google-deepmind/alphafold3/blob/main/docs/input.md#alphafold-server-json-compatibility).

### Out of Memory (OOM) Issue

**Description**:  
Your job may start but fail during execution because it requires more memory than the allocated node can provide. This can occur in both the **datapipeline** and **inference** stages.

**Resolution**:
- Check the **Job Logs** for OOM errors or memory-related failures.
- Modify your code or input data to reduce memory usage.
- Consider requesting **larger node sizes** with higher memory capacity.
