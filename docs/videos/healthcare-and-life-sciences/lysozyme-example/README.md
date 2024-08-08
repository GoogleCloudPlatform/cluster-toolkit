# Lysozyme Example

This example demonstrates a real life case of simulating the Lysozyme protein in
water. It uses the HCLS blueprint to run a multi-step GPU enabled GROMACS run.
This example was featured in
[this YouTube Video](https://youtu.be/kJ-naSow7GQ).

This example has been adapted with changes from tutorials by:

- Justin Lemhul (<http://www.mdtutorials.com>) - licensed under [CC-BY-4.0]
- Alessandra Villa (<https://tutorials.gromacs.org/>) - licensed under [CC-BY-4.0]

[CC-BY-4.0]: https://creativecommons.org/licenses/by/4.0/

> **Note** This example has not been optimized for performance and is meant to
> demonstrate feasibility of a real world example.

## Quota Requirements

The Lysozyme Example only deploys one GPU VM from the blueprint, as such you
will only need quota for:

- GPU: 12 `A2 CPUs` and 1 `NVIDIA A100 GPUs`

Note that these quotas are in addition to the quota requirements for the slurm
login node (2x `N2 CPUs`) and slurm controller VM (4x `C2 CPUs`). The
`spack-builder` VM should have completed and stopped, freeing its CPU quota
usage, before the computational VMs are deployed.

## Instructions

1. Deploy the HCLS blueprint

   Full instructions are found [here](../README.md#deployment-instructions).

1. SSH into the Slurm login node

Go to the
[VM instances page](https://console.cloud.google.com/compute/instances) and you
should see a VM with `login` in the name. SSH into this VM by clicking the `SSH`
button or by any other means.

1. Create a submission directory

   ```bash
   mkdir lysozyme_run01 && cd lysozyme_run01
   ```

1. Copy the contents of this directory into the submission directory

   ```bash
   git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git
   cp -r cluster-toolkit/docs/videos/healthcare-and-life-sciences/lysozyme-example/* .
   ```

1. Copy the Lysozyme protein into the submission directory

   ```bash
   cp /data_input/protein_data_bank/1AKI.pdb .
   ```

1. Submit the job

   Your current directory should now contain

   - the `1AKI.pdb` protein file
   - a `submit.sh` Slurm sbatch script
   - a `config/` directory containing configs used by the run

   The `submit.sh` script contains several steps that are annotated with
   comments. To submit the job call the following command:

   ```bash
   sbatch submit.sh
   ```

1. Monitor the job

   Use the following command to see the status of the job:

   ```bash
   squeue
   ```

   The job state (`ST`) will show `CF` while the job is being configured. Once
   the state switches to `R` the job is running.

   If you refresh the
   [VM instances page](https://console.cloud.google.com/compute/instances) you
   will see an `a2-highgpu-1g` machine that has been auto-scaled up to run this
   job. It will have a name like `hcls01-gpu-ghpc-0`.

   Once the job is in the running state you can track progress with the
   following command:

   ```bash
   tail -f slurm-*.out
   ```

1. Visualize the results

   1. Access the remote desktop using the
      [Chrome Remote Desktop page](https://remotedesktop.google.com/access)
      under Remote devices. If you have not yet set up the remote desktop,
      follow
      [these instructions](../../../../community/modules/remote-desktop/chrome-remote-desktop/README.md#setting-up-the-remote-desktop).
   1. Open a terminal in the remote desktop window.
   1. Navigate to the attached outputs bucket.

      ```bash
      cd /data_output/
      ```

   1. Launch VMD.

      ```bash
      vmd 1AKI_newbox.gro 1AKI_md.xtc
      ```

   1. Update the graphics options:
      1. From the VMD main menu, select `Graphics` > `Representations...` and
         configure the following options in the `Graphical Representations` menu
         that is opened.
      1. Set `Coloring Method` to `Secondary Structure`.
      1. Set `Drawing Method` to `NewCartoon`.
      1. Select the `Trajectory` tab.
      1. Set `Trajectory Smoothing Window Size` to `1`.
      1. Close the `Graphical Representations` menu.
   1. Hit play button in the lower right hand corner of the VMD main.
