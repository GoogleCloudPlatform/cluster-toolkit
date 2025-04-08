# Simple Job Launcher

The Simple Job Launcher is a simple bash file that allows you to submit AlpaFold3 input files
([AlphaFold 3 Input Documentation](https://github.com/google-deepmind/alphafold3/blob/main/docs/input.md))
and allows you to submit it to the data pipeline and/or inference partitions of the AlphaFold 3 blueprint.

## Getting Started
The Simple Job Launcher is preinstalled by the AlphaFold 3 solution. It builds and submits a Slurm sbatch file, which queues your workload with Slurm for execution.

You can find it in the directory `/opt/apps/af3/examples/simple_job_launcher`.

The general syntax for using the Simple Job Launcher is as follows:

```bash
#!/bin/bash

/opt/apps/af3/examples/simple_job_launcher/launch_af3_job.sh --job-type <type> [OPTIONS] <input_path> <output_directory_path> 
```

### Examples
As part of the AlphaFold 3 solution, we include one exemplar input files that you can run with
the Datapipeline step and then take that output and run it with the Inference step.

#### Datapipeline Example

```bash
#!/bin/bash

# set up environment
export PATH=/opt/apps/af3/examples/simple_job_launcher:$HOME
export DATAPIPELINE_OUTPUT=$HOME/datapipeline_output
export INFERENCE_OUTPUT=$HOME/inference_output

# prep your folders
mkdir -p $DATAPIPELINE_OUTPUT
mkdir -p $INFERENCE_OUTPUT

# invoke Datapipeline step of AF3
launch_af3_job.sh --job-type datapipeline /opt/apps/af3/examples/example_inputs/2pv7.json $DATAPIPELINE_OUTPUT

# watch the Slurm queue
while true; do printf "\033[2J\033[H"; squeue; sleep 5; done
```

This should take approximately 10 minutes.

#### Inference Example
This example is meant to run after the Datapipeline example as it requires that step's output as input.

```bash
#!/bin/bash

# environment & folder
# assumes paths are set as in Datapipeline Example and directories created

# invoke inference step of AF3
launch_af3_job.sh --job-type inference $DATAPIPELINE_OUTPUT/2pv7 $INFERENCE_OUTPUT

# watch the Slurm queue
while true; do printf "\033[2J\033[H"; squeue; sleep 5; done
```

This step should take approximately 4 minutes on a g2-standard-16 with 1 L4 GPU.

## Documentation
### Mandatory Arguments
`<type>`, `<input_path>`, and `<output_directory_path>` are mandatory.

The command line argument `--job-type` determines the job type and accepts as input options:

- `datapipeline`
- `inference`

Depending on the job type, the jobs will be submitted to Slurm's data or inference partition. Read
more about the partitions and the architecture in [AlphaFold3 solution](../../README.md). But in short,
Datapipeline jobs run on CPUs with public reference databases loaded to the nodes, and Inference jobs
run on a GPU partition.

`<input_path>` points either to an AlphaFold 3 input json file or a directory of json-files

`<output_directory_path>` configures where AlphaFold3 will write its outputs

> [!TIP]
> The Simple Job Launcher wraps the Datapipeline as well as the Inference steps of the AlphaFold 3 workflow.
> For not getting confused about the different input files required for the two steps, we recommend that you
> provide different input and output paths, depending on the AlphaFold 3 workflow step you want to execute.

### Smart Defaults
The Simple Job Launcher is preconfigured with the AlphFold3 deployment configurations
(such as partition name, database directories, CPU cores to use etc) from your [af3-slurm-deployment.yaml](../../af3-slurm-deployment.yaml). Those defaults are directly encoded into script.

For parameters that you may want to change from invocation to invocation, you can overwrite the relevant
default parameters via the command line, see [Optional Arguments](#optional-arguments).

### Optional Arguments
Options (override defaults):
- `--partition PART`      Slurm partition (Default depends on job type)
- `--mem MEMORY`          Slurm memory request (Default depends on job type)
- `--cpus NUM`            Number of CPUs per task (Default depends on job type)
- `--time SECONDS`        Slurm time limit in TOTAL SECONDS (e.g., 3600 for 1h). (Default depends on job type: '3600's / '3600's)
- `--gres SPEC`           Slurm GPU request (e.g., gpu:1, gpu:a100:1). (Default: none for datapipeline, 'gpu:1' for inference if unset)
- `--job-name-base NAME`  Base name for Slurm job (Default: 'alphafold3')
- `--log-base-dir DIR`    Base directory for logs (Default: '/home/fschuermann_google_com/slurm_logs')
- `--jax-cache-dir DIR`   JAX cache directory (Inference only) (Default: '')
- `-h, --help`            Display this help message and exit.
