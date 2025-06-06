# Simple Service Launcher

The Simple Service Launcher is comprised of a python script that automates the data pipeline and inference
workflows by monitoring a cloud storage bucket for new files and triggering the data
pipeline and/or inference processes on the respective partitions of the AlphaFold 3 blueprint. See a more
detailed description of the logic described below.

There are two options for you to run the service launcher:

1. *Run it directly from a login or controller node* - this mode is useful for testing but relies on the
    user being logged in.

1. *Run it as a demon* - this mode runs the service launcher as a daemon process on the controller node, with
    no interaction required from the user. The process runs as the user `af3` on the slurm controller node. The daemon has its config in `/etc/systemd/system/af3.service` and writes its logs to `/var/log/af3/`. The python `simple_service_launcher.py` script is invoked with the configuration in `/etc/af3config.json`. You can activate the automatic launch of the daemon in the `af3-slurm-deployment.yaml` by setting the variable `af3service_activate: true`.

> [!NOTE]
> Independent of the invocation method, you will have to have a GCS bucket set up to handle inputs
> and outputs. See [Set up Jobs Bucket](#set-up-jobs-bucket).

## Service Logic
There are separate directories for the input to the datapipeline stage of the
AlphaFold 3 workflow and the model inference stage. Any input files that are placed in these folders
will be independently processed. Unless specified differently at script launch, results from the
datapipeline stage will be copied into the input directory of the inference stage.

Here is the detailed logic:

1. **Monitoring:** The launcher monitors the `data_pipeline_toprocess` directory within the specified bucket at a configurable `time-interval` (in seconds).
2. **File Download:** When new files are detected, they are downloaded to the Slurm controller.
3. **Job Submission:** On the controller, files uploaded within the same time window are grouped under a timestamped directory (e.g., `20250312_083336_544/fold_input/slurm_job_ID/fold_input.json`). Each file within the timestamped directory is submitted as an individual Slurm job.
4. **Data Pipeline Execution:** The data pipeline is executed on the Slurm data pipeline partition.
5. **Result Storage:** Upon successful completion, the data pipeline outputs are stored in the `data_pipeline_results` directory within the bucket.
6. **File Archiving:** The input files are moved to either `data_pipeline_success` or `data_pipeline_failed` based on the job's success or failure status.
7. **Inference Trigger (Optional):** If enabled, the data pipeline results are automatically copied to the `inference_toprocess` directory, triggering the inference process.
8. **Inference Execution:** The inference workflow follows the same file download, job submission, result storage, and file archiving logic as the data pipeline.
9. **Continuous Monitoring:** The daemon continues to monitor the bucket for new files.

## Setup Instructions
### Setup without daemon
> [!NOTE]
> Follow this section if you want to experiment with the service launcher from the command line. If you want
> to run a daemon for the service launcher, follow the instructions in the next section below.

If you want to experiment with different configurations for the service launcher, it can be useful
to launch it from the login prompt of the controller node in the cluster. Once you have SSHed
into the controller node (e.g. in console.cloud.google.com look for Compute Engine > VM Instances, and
identify the controller VM `[your deployment name]-controller` and click `SSH` to log in via the web browser),
you can launch the script as follows:

```bash
/opt/apps/af3/venv/bin/activate
python3 /opt/apps/af3/examples/simple_service_launcher/simple_service_launcher.py --config-file /etc/af3config.json
```

This will run the script with the default parameters initialized from the `#AF3 Simple Service` section in
the [af3-slurm-deployment.yaml](../../af3-slurm-deployment.yaml).

> [!WARNING]
> While you can change the input/output/intermediate folders, it is expecting these folders to live
> on a GCS bucket. Follow instructions in section [Prerequisites](#prerequisites) to set up a bucket to
> receive input and deposit results.

The log information will be printed in the console and can be found in `/var/log/af3/`.
See section [Configuration Variants](#configuration-variants) for all the possible customizations.

### Setup with daemon
> [!NOTE]
> By default, the service launcher is not started as a daemon by default. If you want to run the daemon,
> follow the instructions in this section. If you want to launch it interactively from the login prompt,
> follow the instructions in the previous section.

1. Set up Jobs Bucket
   Follow instructions in section [Prerequisites](#prerequisites) to set up a bucket to receive
   input and deposit results.

1. Activate it in the Blueprint

   In the file [af3-slurm-deployment.yaml](../../af3-slurm-deployment.yaml) set the variable
   `af3service_activate: true`, **before you deploy the cluster**. This will launch the `af3.service` on the
   controller node at startup. The daemon will be run as user `af3` on the controller node.

1. Customize the Service
   The simple service launcher comes with meaningful presets, but you may want to customize it.
   For example, the launcher allows you to configure which stages to execute, different directories,
   change the default Slurm partition names, set time-out durations, or other variables related to
   the compute resources given to each job.

   The configuration parameters for the service launcher script are located on the controller node in
   `/etc/af3config.json`. If you want to modify the launcher configuration at runtime, you can
   modify that file and restart the service from the controller node:

   ```bash
   sudo systemctl stop af3
   sudo systemctl daemon-reload
   sudo systemctl start af3
   ```

   You can check the logs of the service in `/var/log/af3/`.

   The initial values for the config file are populated as part of the deployment and you can change
   these parameters in the `AF3 Service settings` section of the  [af3-slurm-deployment.yaml](../../af3-slurm-deployment.yaml) file.

## Configuration Variants

By default, the script executes both the data pipeline and inference stages. To execute only one stage, use the following command-line arguments:

* `--run-inference='False'`: Executes only the data pipeline.
* `--run-data-pipeline='False'`: Executes only the inference.

**Example:**

```bash
python your_script.py --bucket-name your-bucket  --run-inference='False'
```

**Note:** You can customize the directory names used within the bucket (e.g., `data_pipeline_toprocess`, `inference_toprocess`, etc.) by using the corresponding command-line arguments listed below. This allows you to adapt the script to your specific bucket structure and naming conventions.

### Full list of Command-line Arguments

#### Bucket Name and Config File
`--bucket-name`: your bucket name
`--config-file`: file to set all these argument
#### Data Pipeline and Inference Options
`--run-data-pipeline`: set to `True` to execute the data pipeline. Set to `False` to skip the pipeline execution
`--run-inference`: set to `True` to run the inference process. Set to `False` to skip running inference
#### Time Interval
`--time-interval`: specifies the time interval (in seconds) for the Python daemon to check for input files
#### Timeout
`--inference-job-timeout`: inference timeout (seconds)
`--data-pipeline-job-timeout`: data pipeline timeout (seconds)
`--pending-job-time-limit`: Check for pending jobs older than this time limit (in seconds). If not provided, no time limit is applied.
#### Data Pipeline Bucket Directories
`--data-pipeline-bucket-input-dir`: bucket directory for data pipeline input files
`--data-pipeline-bucket-running-dir`: bucket directory for running data pipelines
`--data-pipeline-bucket-done-dir`: bucket directory for successful data pipelines
`--data-pipeline-bucket-failed-dir`: bucket directory for failed data pipelines
`--data-pipeline-bucket-result-dir`: bucket directory for storing data pipeline results
#### Inference Bucket Directories
`--inference-bucket-input-dir`: bucket directory for inference input files
`--inference-bucket-running-dir`: bucket directory for running inferences
`--inference-bucket-done-dir`: bucket directory for successful inferences
`--inference-bucket-failed-dir`: bucket directory for failed inferences
`--inference-bucket-result-dir`: bucket directory for storing inference results
#### Local Directories (Slurm VM)
`--local-data-pipeline-dir`: local directory for data pipelines
`--local-inference-dir`: local directory for inferences
`--sif-dir`: local directory for Singularity/Apptainer image (The image should be named as `af3.sif`)
`--db-dir`: local directory for databases
`--model-dir`: local directory for model file (model weight)
`--pdb-database-path`: local directory for data-pipeline pdb database
#### Slurm Partition
`--data-pipeline-partition-name`: datapipeline slurm partition name
`--data-pipeline-partition-memory-size`: data pipeline slurm partition memory size (GB)
`--data-pipeline-partition-cpu-count`: data pipeline slurm partition cpu count
`--inference-partition-name`: inference slurm partition name
`--inference-partition-memory-size`: inference slurm partition memory size (GB)
`--inference-partition-cpu-count`: inference slurm partition cpu count
`--auto-calculate-resource`: calculate required resource based on input sequence length
#### Scientific Inference Parameters
`--inference-max-template-date`: Maximum template release date to consider. Format: YYYY-MM-DD
`--inference-conformer-max-iterations`:  Optional override for maximum number of iterations to run for RDKit conformer search
`--inference-num-recycles`: Number of recycles to use during inference. Lower bound=1
`--inference-num-diffusion-samples`: Number of diffusion samples to generate. Lower bound=1
`--inference-num-seeds`: Number of seeds to use for inference.
`--inference-save-embeddings` : Whether to save the final trunk single and pair embeddings in the output
See AF3 [run_alphafold.py](https://github.com/google-deepmind/alphafold3/blob/main/run_alphafold.py) for more details.

## Prerequisites
### Set up Jobs Bucket

If you want to use the simple service launcher, you need to create an additional bucket, that should
be located in the region where you stand up your cluster:

```bash
#!/bin/bash

UNIQUE_JOB_BUCKET=<your-bucket>
PROJECT_ID=<your-gcp-project>
REGION=<your-preferred-region>

gcloud storage buckets create gs://${UNIQUE_JOB_BUCKET} \
    --project=${PROJECT_ID} \
    --default-storage-class=STANDARD --location=${REGION} \
    --uniform-bucket-level-access
```

This bucket will serve as the interface to the service. This is where you place input files
that should be run through the datapipeline or inference pipeline.

You can configure the structure of this bucket in the command-line arguments to the
script or config file. Here's what you would execute to get the default folder structure:

By default, input to the datapipeline step should be deposited in as

```bash
gsutil cp your-folding.json gs://[your-globally-unique-af3-jobs-bucket]/data_pipeline_toprocess
```

Results will be found in

```bash
gs://[your-globally-unique-af3-jobs-bucket]/data_pipeline_results
```

For inference, jobs should be deposited in

```bash
gsutil cp your-folding.json gs://[your-globally-unique-af3-jobs-bucket]/inference_toprocess
```

Results will be found in

```bash
gs://<your-globally-unique-af3-jobs-bucket>/inference_results
```

By default, the service will move results from the data pipeline step results folder into the
inference input folder.

Other useful directories on this bucket (configurable in the af3 service config file):

```bash
gs://<your-globally-unique-af3-jobs-bucket>/data_pipeline_toprocess/
gs://<your-globally-unique-af3-jobs-bucket>/data_pipeline_running/
gs://<your-globally-unique-af3-jobs-bucket>/date_pipeline_success/
gs://<your-globally-unique-af3-jobs-bucket>/data_pipeline_failed/
gs://<your-globally-unique-af3-jobs-bucket>/date_pipeline_results/

gs://<your-globally-unique-af3-jobs-bucket>/inference_toprocess/
gs://<your-globally-unique-af3-jobs-bucket>/inference_running/
gs://<your-globally-unique-af3-jobs-bucket>/inference_success/
gs://<your-globally-unique-af3-jobs-bucket>/inference_failed/
gs://<your-globally-unique-af3-jobs-bucket>/inference_results/
```
