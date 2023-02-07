# Factor Xa Example

This example demonstrates a real life example of simulating the Factor Xa
protein in water.

> **Note** This example has not been optimized for performance and is meant to
> demonstrate feasibility of a real world example

## Instructions

1. Deploy the HCLS blueprint

   Full instructions are found [here](../README.md).

1. SSH into the Slurm login node

1. Create a submission directory

   ```bash
   mkdir factorxa_run01 && cd factorxa_run01
   ```

1. Copy the contents of this directory into the submission directory

   ```bash
   git clone https://github.com/GoogleCloudPlatform/hpc-toolkit.git
   cp -r hpc-toolkit/docs/videos/healthcare-and-life-sciences/Factor-Xa-example/* .
   ```

1. Copy the Factor Xa protein into the submission directory

   ```bash
   cp /data_input/protein_data_bank/1FJS.pdb .
   ```

1. Submit the job

   ```bash
   sbatch submit.sh
   ```
