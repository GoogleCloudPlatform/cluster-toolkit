# Simple Ipynb Launcher
The Simple Ipynb Launcher is a Jupyter Notebook-based interface designed to run folding jobs on the Alphafold 3 High Throughput solution. We are using Google's Vertex AI Workbench services to host the Jupyter notebook and configure it to interoperate with the Slurm cluster via the Slurm REST API. For convenience, we provide a Jupyter notebook that can run folding jobs and analyze and visualize the output.

## Setup Guide: Jupyter Notebook with SLURM
Please note that the launcher needs 2 specific setup steps:

- **[Setup AF3 with Simple Ipynb Launcher - PART 1: specific cluster settings](./Setup-pre-cluster-deployment.md)**: Instructions to be followed before bringing up the `af3-slurm.yaml` Slurm cluster.

- **[Setup: Setup AF3 with Simple Ipynb Launcher - PART 2: IPython notebook setup](./Setup-post-cluster-deployment.md)**: Instructions for launching the notebook environment.

## Usage Guide
For usage of the Ipynb Launcher consult the [Step-by-Step Instructions](./Ipynb.md)

## Known Issues & How to Fix Them
You may encounter the following problems while using the notebook. Each issue includes specific symptoms, causes, and actionable resolutions.

### Warning during dependency installation

#### Symptom
During Python dependency installation, you see a warning like:

```text
ERROR: pip's dependency resolver does not currently take into account all the packages that are installed. ...
google-cloud-bigtable 1.7.3 requires grpc-google-iam-v1<0.13dev,>=0.12.3, but you have grpc-google-iam-v1 0.14.2 which is incompatible
```

#### Cause
Pip detects a version mismatch between `grpc-google-iam-v1` and `google-cloud-bigtable`.

#### Resolution
**Ignore This Warning**. This package `google-cloud-bigtable` is not critical to the notebook’s core functionality. There is no known runtime impact or errors caused by this mismatch.

### Resource Unavailability (Out of Capacity)

#### Symptom
Your Slurm job is queued but not running for an unusually long time.

#### Cause
Google Cloud Platform (GCP) is unable to provision the requested compute resources due to temporary regional or zone capacity shortages.

#### Resolution

1. **Check the Slurm Controller Node Logs**:
   - Take a look at the **VM log of the Slurm controller node**.
     You can access the log through two methods:
      1. Using the Google Cloud Console (VM UI)
         - Open the Google Cloud Console: [https://console.cloud.google.com/](https://console.cloud.google.com/)
         - At the top of the page, make sure you have selected the correct project name.
         - In the left-hand menu, navigate to **Compute Engine** > **VM instances**.
         - Click on the name of your Slurm controller VM in the list.
         - On the VM details page, scroll down to the **Logs** section and click **Logging**.
         - You will be able to view the relevant logs from there.

      2. Accessing the Slurm Controller Node Log (via SSH or Google Cloud Console)

         You can access the Slurm Controller Node either through your local terminal or the Google Cloud Console:

         - Option A: SSH from Terminal

            Run this command in your local terminal (replace the placeholders with your actual instance name and zone):

            ```bash
            gcloud compute ssh [INSTANCE_NAME] --zone=[ZONE]
            ```

         - Option B: SSH from Google Cloud Console
            1. Go to the VM instances page.
            2. Find your Slurm Controller VM.
            3. Click the SSH button next to it to open a web-based terminal session.

         After connecting to the Slurm Controller VM (via SSH or the Google Cloud Console), open the terminal session within that VM and run the following command:

            ```bash
            sudo cat /var/log/slurm/slurmctld.log
            ```

            Why we use `sudo`?

            The `slurmctld.log` file is typically owned by the `root` user and is not readable by standard (non-root) users. The `sudo` command temporarily elevates your privileges, allowing you to access files that require administrative permissions.

            Without `sudo`, attempting to read the log file may result in a **"Permission denied"** error.

   - Look for error messages related to resource availability. In cases of insufficient capacity, you may see a message like:

     ```text
     GCP Error: Zone does not currently have sufficient capacity for resources
     ```

2. **Wait and Retry**:
   - This issue is often **temporary**.
   - Wait **5–10 minutes**, then re-run the `get_job_state` cell in your notebook to check if the job has started.

3. **Consider Changing Region or Zone (If Possible)**:
   - If the issue persists, consider reconfiguring your cluster to use a different **GCP region or zone** with more available capacity.

4. **Scale Down Other Jobs**:
   - If you're running many concurrent jobs, free up resources by canceling or pausing non-critical workloads.

5. **Contact Your Cloud Admin (If Applicable)**:
   - If you're working in a managed environment, consult your GCP or cluster administrator for assistance with quotas or reserved capacity.

### Node Creation In Progress

#### Symptom  
Job status shows error state such as `NODE_FAIL` shortly after submission from the notebook, especially on first run or after cluster scale-down.

#### Cause
Slurm is waiting for new compute nodes to finish provisioning and become available.

#### Resolution
1. **Verify the Issue**:
   - Check the **VM log of the Slurm controller node** to review provisioning-related messages.
   - Look for indicators that nodes are still being created or initialized.

2. **Wait for Node Readiness**:
   - Allow **5–10 minutes** for the compute nodes to finish provisioning and register with the cluster.
   - After waiting, check your job status using:
     - The `squeue` command on the controller node, or
     - The `get_job_state` cell in your notebook.

### Input File Format Error

#### Symptom
Your job fails immediately or throws input parsing errors.

#### Cause
The input file may be invalid due to one of the following reasons:
- It is in an **unsupported format**
- It is **corrupted**
- It is **missing required fields**

#### Resolution

1. **Check the Job Logs** for specific error messages on the Slurm controller node:

   - **Data Pipeline Log Folder**:

     ```text
     /home/af3ipynb/datapipeline_result/<input_file>/<timestamp>/slurm_logs/job_<job_id>/
     ```

   - **Inference Log Folder**:

     ```text
     /home/af3ipynb/inference_result/<input_file>/<timestamp>/slurm_logs/job_<job_id>/
     ```

   Each log folder contains:
   - `err.txt`: captures **stderr** (standard error)
   - `out.txt`: captures **stdout** (standard output)

2. **Validate the Input File**:

   Ensure the input format matches one of the supported types:
   - `alphafoldserver`
   - `alphafold3`

   Refer to the [Alphafold Input Requirements](https://github.com/google-deepmind/alphafold3/blob/main/docs/input.md#alphafold-server-json-compatibility) for complete format specifications.

3. **Fix and Resubmit**:
   - Correct any formatting issues or regenerate the input file.
   - Re-upload the corrected file.
   - Resubmit your job.

### Out of Memory (OOM) Issue

#### Symptom
Your job starts but fails during execution with **Out of Memory (OOM)** errors.

#### Cause
The job requires **more memory** than is available on the assigned compute node(s). This can happen during either the **data pipeline** or **inference** stages.

#### Resolution

1. **Check Job Logs on the Slurm Controller Node for OOM Errors**:

   - Look in the following files for memory-related error messages:

     - **Data Pipeline**:

          ```text
          /home/af3ipynb/datapipeline_result/<input_file>/<timestamp>/slurm_logs/job_<job_id>/
          ```

     - **Inference**:

          ```text
          /home/af3ipynb/inference_result/<input_file>/<timestamp>/slurm_logs/job_<job_id>/
          ```

     Each log folder contains:
     - `err.txt`: captures **stderr** (standard error)
     - `out.txt`: captures **stdout** (standard output)

   - Common error messages include:

      ```bash
      # Data pipeline
      Out of Memory (OOM)
      ```

      or

      ```bash
      # Inference
      RuntimeError: CUDA out of memory
      Killed: 9
      MemoryError
      ```

2. **Handle GPU Memory Constraints**:

   - Enable the following setting to optimize GPU memory usage:
     - `inference_enable_unified_memory`:  
      This setting allows the system to use unified memory, enabling the GPU to access system (CPU) RAM when its own memory is insufficient. This can help reduce the likelihood of out-of-memory (OOM) errors during inference. To enable this feature, set the value to `true` in your `af3-slurm-deployment.yaml` file:

        ```json
        "inference_enable_unified_memory": true
        ```

3. **Request More Memory**:

   - You can modify your **data pipeline** or **inference** job submission script or cluster configuration to request nodes with higher memory capacity.

   - For example, in the `af3-slurm-deployment.yaml` file, if the `inference_g2_partition` memory value is currently set to `46`, you can increase it to `50` to request more memory:

      ```yaml
      inference_g2_partition:
        memory: 50
      ```

      > [!WARNING]
      > Before increasing the memory value, check the maximum memory capacity available for the node type to avoid misconfiguration.

   - Alternatively, you can specify memory directly in your Slurm job script:

      ```bash
      #SBATCH --mem=50G
      ```

   - You may also consider exploring more powerful machine types or configurations that offer more memory or better performance.

### Notebook Save Error

  <img src="adm/notebook-save-failed.png" alt="notebook save failed" width="1000">

#### Symptom
The notebook file fails to save, and an error message appears. This can also happen suddenly due to an automated save triggered by Jupyter.

#### Cause
This issue can occur due to network connectivity problems, insufficient disk space, or lack of write permissions in the save location.

#### Resolution

- Verify your network connection is stable.

- Ensure you have write permissions to the save directory.

- Check that there is enough disk space available.

- Try saving the notebook again manually.

- To avoid losing work, save your current progress manually to a different file before retrying. This way, you can decide whether to overwrite the original notebook.

- You may choose to **ignore this error**, but note that unsaved changes could be lost.
