# Lysozyme Example

This example demonstrates a real life case of simulating the Lysozyme protein
in water. It uses the HCLS blueprint to run a multi-step GPU enabled gromacs
run.

This example has been adapted with changes from tutorials by:

- Justin Lemhul (http://www.mdtutorials.com) - licensed under [CC-BY-4.0]
- Alessandra Villa (https://tutorials.gromacs.org/) - licensed under [CC-BY-4.0]

[CC-BY-4.0]: https://creativecommons.org/licenses/by/4.0/

> **Note** This example has not been optimized for performance and is meant to
> demonstrate feasibility of a real world example.

## Instructions

1. Deploy the HCLS blueprint

   Full instructions are found [here](../README.md).

1. SSH into the Slurm login node

1. Create a submission directory

   ```bash
   mkdir lysozyme_run01 && cd lysozyme_run01
   ```

1. Copy the contents of this directory into the submission directory

   ```bash
   git clone https://github.com/GoogleCloudPlatform/hpc-toolkit.git
   cp -r hpc-toolkit/docs/videos/healthcare-and-life-sciences/lysozyme-example/* .
   ```

1. Copy the Lysozyme protein into the submission directory

   ```bash
   cp /data_input/protein_data_bank/1AKI.pdb .
   ```

1. Submit the job

   ```bash
   sbatch submit.sh
   ```
