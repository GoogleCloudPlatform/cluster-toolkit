# Building Apptainer (SIF) Images Interactively

The [builder.yaml](builder.yaml) HPC Toolkit blueprint creates a Google Cloud instance with Apptainer installed that you can use to interactively build SIF images.

## Deploy

Make a copy of the `build.yaml` file, e.g., 

```cp build.yaml mybuild.yaml```

Then edit it to set the `project_id` field appropriately. Use your preferred text editor or the `sed` command below to make the change.

```bash
sed -i s/_YOUR_GCP_PROJECT_ID_/${PROJECT_ID}/g builder.yaml
```

Now you can create the deployment artifacts with the command

```./ghpc create mybuild.yaml```

You should see output that looks like

```
To deploy your infrastructure please run:

./ghpc deploy bldtainer

Find instructions for cleanly destroying infrastructure and advanced manual
deployment instructions at:

bldtainer/instructions.txt
```

Enter ```./ghpc deploy bldtainer``` to deploy the Apptainer build instance.

## Develop

### Log In and Verify

Log into the build instance using the command

```bash
gcloud compute ssh bldtainer-0 --zone us-central1-a --tunnel-through-iap
```

On the build instance check to ensure Apptainer is installed using the command

```bash
apptainer
```

You should see output that looks like

```
apptainer 
Usage:
  apptainer [global options...] <command>

Available Commands:
  build       Build an Apptainer image
  cache       Manage the local cache
  capability  Manage Linux capabilities for users and groups
  checkpoint  Manage container checkpoint state (experimental)
  completion  Generate the autocompletion script for the specified shell
  config      Manage various apptainer configuration (root user only)
  delete      Deletes requested image from the library
  exec        Run a command within a container
  inspect     Show metadata for an image
  instance    Manage containers running as services
  key         Manage OpenPGP keys
  oci         Manage OCI containers
  overlay     Manage an EXT3 writable overlay image
  plugin      Manage Apptainer plugins
  pull        Pull an image from a URI
  push        Upload image to the provided URI
  remote      Manage apptainer remote endpoints, keyservers and OCI/Docker registry credentials
  run         Run the user-defined default command within a container
  run-help    Show the user-defined help for an image
  search      Search a Container Library for images
  shell       Run a shell within a container
  sif         Manipulate Singularity Image Format (SIF) images
  sign        Add digital signature(s) to an image
  test        Run the user-defined tests within a container
  verify      Verify digital signature(s) within an image
  version     Show the version for Apptainer

Run 'apptainer --help' for more detailed usage information.
```

### Create a Simple SIF Image

Apptainer builds SIF images from a [definition file](https://apptainer.org/user-docs/3.8/definition_files.html). Use this command to create a definition file for a simple SIF image

```bash
cat <<- "EOF" > lolcow.def
Bootstrap: docker
From: ubuntu:20.04

%post
    apt-get update -y
    apt-get -y install cowsay lolcat

%environment
    export LC_ALL=C
    export PATH=/usr/games:$PATH

%runscript
    date | cowsay | lolcat
EOF
```

Now use the `apptainer build` command to create a SIF image

```bash
apptainer build lolcow.sif lolcow.def
```

This will generate many lines of output

```
INFO:    User not listed in /etc/subuid, trying root-mapped namespace
INFO:    The %post section will be run under fakeroot
INFO:    Starting build...
Getting image source signatures
Copying blob 96d54c3075c9 done  
Copying config bf40b7bc7a done  
Writing manifest to image destination
Storing signatures
2023/11/08 20:33:20  info unpack layer: sha256:96d54c3075c9eeaed5561fd620828fd6bb5d80ecae7cb25f9ba5f7d88ea6e15c
INFO:    Running post scriptlet

... many more lines ...

Running hooks in /etc/ca-certificates/update.d...
done.
INFO:    Adding environment to container
INFO:    Adding runscript
INFO:    Creating SIF file...
INFO:    Build complete: lolcow.sif
```

You can execute the `lolcow.sif` file via the `apptainer run` command

```bash
apptainer run lolcow.sif
```

or just enter `./lolcow.sif`

In either case you should see output similar to

```
< Wed Nov 8 20:36:57 UTC 2023 >
 -----------------------------
        \   ^__^
         \  (oo)\_______
            (__)\       )\/\
                ||----w |
                ||     ||
```

## Publish

Containers package software and dependencies so that they can be easily shared and deployed. One of the most effective ways to share containers is through _repositories_. Google Cloud provides [Artifact Registry](https://cloud.google.com/artifact-registry) which stores, manages, and secures build artifacts - including containers. SIF images can be stored in Artifact Registry using the [OCI Registry As Storage](https://oras.land/) (oras) scheme. Slurm jobs running in an HPC Toolkit deployed cluster can pull the SIF images they need from Artifact Registry as they need them.

If you don't have an Artifact Registry repository available follow the steps [here](https://cloud.google.com/artifact-registry/docs/repositories/create-repos#description) to create one. Then create an environment variable for your repository URL

```bash
export REPOSITORY_URL=<ARTIFACT REGISTRY REPOSITORY URL> # e.g. oras://us-docker.pkg.dev/myproject/sifs
```

Apptainer needs to authenticate to your repository before it can push or pull images. Use this command to authenticate

```bash
apptainer remote login \
--username=oauth2accesstoken \
--password=$(gcloud auth print-access-token) \ 
${REPOSITORY_URL}
```

You should see output like

```
INFO:    Token stored in /home/joeuser/.apptainer/remote.yaml
```

Push the `lolcow.sif` container image to Artifact Registry using the command

```bash
apptainer push lolcow.sif ${REPOSITORY_URL}/lolcow:1.0
```

Which will generate out put like

```
82.6MiB / 82.6MiB [================================================================================] 100 % 141.8 MiB/s 0s
82.6MiB / 82.6MiB [================================================================================] 100 % 141.8 MiB/s 0s
INFO:    Upload complete
```

Now you can delete the SIF image you built with

```bash
rm lolcow.sif
```

and then retrieve it using the command

```bash
apptainer pull ${REPOSITORY_URL}/lolcow:1.0
```

with output similar to

```
INFO:    Downloading oras image
403.0b / 403.0b [===============================================================================================] 100 %0s
82.6MiB / 82.6MiB [================================================================================] 100 % 105.0 MiB/s 0s
```

The image you pulled with be named `lolcow_1.0.sif`

## Cleanup

When you have finished defining and building containers log out of the `bldtainer-0` instance and destroy it using with

```bash
./ghpc destroy bldtainer
```

