# Visual Studio Code in a Container

[Visual Studio Code](https://code.visualstudio.com/) (vscode) is a popular, powerful, freely available IDE. Developers generally use an instances of `vscode` installed on their local workstations. There are, however, situations where running it on a login or compute node of an HPC system could be advantageous. For example, the need to use a more powerful GPU or have access to more compute cores than a workstation provides.

This example demonstrates packaging `vscode` in an Apptainer container, deploying the container to a compute node in a Slurm-based HPC system created using the [Cloud HPC Toolkit](https://cloud.google.com/hpc-toolkit/docs/overview), and connecting to it via the `vscode` [Remote Tunnels](https://code.visualstudio.com/docs/remote/tunnels) extension.

### Before you begin
This demonstration assumes you have access to an [Artifact Registry](https://cloud.google.com/artifact-registry) repository and that you have set up the Apptainer custom build step. See [this section](../../../README.md#before-you-begin) for details.

## Container Definition

The [vscode.def](./vscode.def) file defines the construction of the container by the `apptainer build` command.

The build uses an `ubuntu` base image and then does a standard `vscode` installation

```
%post
    apt update -y
    apt install -y wget gpg
    wget -qO- https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor > packages.microsoft.gpg
    install -D -o root -g root -m 644 packages.microsoft.gpg /etc/apt/keyrings/packages.microsoft.gpg
    sh -c 'echo "deb [arch=amd64,arm64,armhf signed-by=/etc/apt/keyrings/packages.microsoft.gpg] https://packages.microsoft.com/repos/code stable main" > /etc/apt/sources.list.d/vscode.list'
    rm -f packages.microsoft.gpg
    apt install -y apt-transport-https
    apt update -y
    apt install -y code
```

## Container Build

You build the `vscode` container and save it to Artifact Registry with the command

```bash
gcloud builds submit --config=vscodebuild.yaml
```

Note that this will used the default values for
- _LOCATION: _*us-docker.pkg.dev*_
- _REPOSITORY: _*sifs*_
- _VERSION: _*latest*_

If you want to change any of these values add the `--substitution` switch to the command above, e.g., to set the version to `1.84`

```bash
gcloud builds submit --config=vscodebuild.yaml --substitutions=_VERSION=1.84
```

## Usage

To use the `vscode` container you built, deploy a Slurm-based HPC System using the [slurm-apptainer.yaml](../../../cluster/slurm-apptainer.yaml) blueprint following the process described [here](../../../cluster/README.md). Login to the HPC system's login node with the command

```bash
gcloud compute ssh \
  $(gcloud compute instances list \
      --filter="NAME ~ login" \
      --format="value(NAME)") \
  --tunnel-through-iap
```

Set up access to the Artifact Registry repository

```bash
export REPOSITORY_URL=#ARTIFACT REGISTRY REPOSITORY URL# e.g. oras://us-docker.pkg.dev/myproject/sifs
```

```bash
apptainer remote login \
--username=oauth2accesstoken \
--password=$(gcloud auth print-access-token) \ 
${REPOSITORY_URL}
```

The command

```bash
srun -N1 apptainer run oras://us-docker.pkg.dev/wkh-as-vpc-fluxfw/sifs/vscode:latest code tunnel
```

allocates a compute node then downloads the `vscode` container and uses it to execute the command `code tunnel` which sets up the remote side of the VS Code tunnel. You should see output similar to

```
INFO:    Downloading oras image
*
* Visual Studio Code Server
*
* By using the software, you agree to
* the Visual Studio Code Server License Terms (https://aka.ms/vscode-server-license) and
* the Microsoft Privacy Statement (https://privacy.microsoft.com/en-US/privacystatement).
*
[2023-11-15 17:14:59] info Using Github for authentication, run `code tunnel user login --provider <provider>` option to change this.
To grant access to the server, please log into https://github.com/login/device and use code XXXX-YYYY
```

[This](https://code.visualstudio.com/docs/remote/tunnels) document explains how to use the [Remote Explorer](https://marketplace.visualstudio.com/items?itemName=ms-vscode.remote-explorer) extension to connect to your remote instance.

## Cleanup

When your editing session is complete use the Slurm `squeue` command to get the `job id` associated with it and then use `scancel` to terminate the job.