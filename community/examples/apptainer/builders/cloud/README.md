# Building Apptainer (SIF) Images with Cloud Build

[Cloud Build](https://cloud.google.com/build?hl=en) is Google's serverless CI/CD platform. It provides a simple extension mechanism you can use to support creation of SIF images using Google Cloud resources rather than spinning up a dedicated instance.

## Apptainer Build Step

Before you can use Cloud Build to create SIF images you must add a [custom build step](https://cloud.google.com/build/docs/configuring-builds/use-community-and-custom-builders) to your project. Cloud Build custom build steps are just [Docker](https://www.docker.com/) images that package up the software required to perform some action, in this case running Apptainer. You use Cloud Build to build those custom build step images and make them available in your project. The Apptainer [Dockerfile](buildstep/Dockerfile) and [config](buildstep/cloudbuild.yaml) are in the `buildstep` directory.

To setup the Apptainer build step navigate to the `buildstep` directory

```bash
cd buildstep
```

The use this command to use Cloud Build to add the Apptainer build step to your project

```bash
gcloud builds submit --config cloudbuild.yaml .
```

You should see output similar to

```
Creating temporary tarball archive of 2 file(s) totalling 506 bytes before compression.
Uploading tarball of [.] to [gs://myproject_cloudbuild/source/1699547428.666519-093b1756b5e74774b02660be6fe56e81.tgz]
Created [https://cloudbuild.googleapis.com/v1/projects/myproject/locations/global/builds/7e2e8314-d4b3-4366-a1d0-529c7585f4e5].
Logs are available at [ https://console.cloud.google.com/cloud-build/builds/7e2e8314-d4b3-4366-a1d0-529c7585f4e5?project=308784392426 ].
----------------------------------------------------------------- REMOTE BUILD OUTPUT -----------------------------------------------------------------
starting build "7e2e8314-d4b3-4366-a1d0-529c7585f4e5"

FETCHSOURCE
Fetching storage object: gs://myproject_cloudbuild/source/1699547428.666519-093b1756b5e74774b02660be6fe56e81.tgz#1699547429693674
Copying gs://myproject_cloudbuild/source/1699547428.666519-093b1756b5e74774b02660be6fe56e81.tgz#1699547429693674...
/ [1 files][  513.0 B/  513.0 B]                                                
Operation completed over 1 objects/513.0 B.                                      
BUILD
Already have image (with digest): gcr.io/cloud-builders/docker
Sending build context to Docker daemon  3.072kB
Step 1/4 : FROM rockylinux/rockylinux:9
9: Pulling from rockylinux/rockylinux
4031b0359885: Pulling fs layer
4031b0359885: Verifying Checksum
4031b0359885: Download complete
4031b0359885: Pull complete
Digest: sha256:984ef1ce766960f62ee3caebf316ff96a5c8190d1095258f670ee5da1afdf47e
Status: Downloaded newer image for rockylinux/rockylinux:9
 ---> 175264fac6da
Step 2/4 : RUN     dnf install -y epel-release && dnf install -y apptainer-suid

... many more lines ...

Complete!
Removing intermediate container e375d0a14141
 ---> ffb8c6ea80b6
Step 3/4 : RUN     apptainer config fakeroot --add root
 ---> Running in ebe9e8913ce4
Removing intermediate container ebe9e8913ce4
 ---> 449ace923fed
Step 4/4 : ENTRYPOINT ["apptainer"]
 ---> Running in 6848ff611745
Removing intermediate container 6848ff611745
 ---> c15339811c18
Successfully built c15339811c18
Successfully tagged gcr.io/wkh-as-vpc-fluxfw/apptainer:latest
PUSH
Pushing gcr.io/myproject/apptainer:latest
The push refers to repository [gcr.io/myproject/apptainer]
a96c37b03128: Preparing
6b0dea74f889: Preparing
bb25ee446163: Preparing
a96c37b03128: Pushed
bb25ee446163: Pushed
6b0dea74f889: Pushed
latest: digest: sha256:b698fb2291d705feada2cc92c26617743dbb81c5e8066d249163a67d923e32cb size: 949
DONE
-------------------------------------------------------------------------------------------------------------------------------------------------------
ID                                    CREATE_TIME                DURATION  SOURCE                                                                                           IMAGES                                        STATUS
7e2e8314-d4b3-4366-a1d0-529c7585f4e5  2023-11-09T16:30:30+00:00  1M7S      gs://myproject_cloudbuild/source/1699547428.666519-093b1756b5e74774b02660be6fe56e81.tgz  gcr.io/myproject/apptainer (+1 more)  SUCCESS
```

You can use the following command to verify that container for the Apptainer build step is present

```bash
gcloud container images list --repository gcr.io/#PROJECT_NAME#
```

The output should look like

```bash
NAME
gcr.io/#PROJECT_NAME#/apptainer
```

## Building

Containers package software and dependencies so that they can be easily shared and deployed. One of the most effective ways to share containers is through _repositories_. Google Cloud provides [Artifact Registry](https://cloud.google.com/artifact-registry) which stores, manages, and secures build artifacts - including containers. SIF images can be stored in Artifact Registry using the [OCI Registry As Storage](https://oras.land/) (oras) scheme. Slurm jobs running in an HPC Toolkit deployed cluster can pull the SIF images they need from Artifact Registry as they need them.

If you don't have an Artifact Registry repository available follow the steps [here](https://cloud.google.com/artifact-registry/docs/repositories/create-repos#description) to create one.

To build a SIF image you need an Apptainer [definition file](https://apptainer.org/docs/user/latest/definition_files.html) and a Cloud Build [build configuration file](https://cloud.google.com/build/docs/build-config-file-schema). In this example you will use the [lolcow.def](./lolcow.def) definition file and the [lolcow.yaml](./lolcow.yaml) build configuration.

The `lolcow.def` definition file specifies an [Ubuntu](https://ubuntu.com/) 20.04 based image with the [cowsay](https://pypi.org/project/cowsay/) and [lolcat](https://manpages.ubuntu.com/manpages/focal/man6/lolcat.6.html) commands installed. At runtime the image uses `cowsay` and `lolcat` to print the current date.

The `lolcow.yaml` build configuration file uses the Apptainer build step to create a SIF image and then push it to Google [Artifact Registry](https://cloud.google.com/artifact-registry).

To create a _lolcow_ SIF image and push it to an Artifact Registry repository execute the command

```bash
gcloud builds submit --config lolcow.yaml --substitutions=_VERSION=1.0,_REPOSITORY=#REPOSITORY_NAME# .
```

where *REPOSITORY_NAME* is the name of an Artifact Registry repository in your project to which you have write access.

The output will be similar to

```
Creating temporary tarball archive of 5 file(s) totalling 1.2 KiB before compression.
Uploading tarball of [.] to [gs://wkh-as-vpc-fluxfw_cloudbuild/source/1699553041.598345-dc266633efc0492e861e00f570ed15b1.tgz]
Created [https://cloudbuild.googleapis.com/v1/projects/myproject/locations/global/builds/07b6a504-71e5-4ab8-b666-628a450ce62f].
Logs are available at [ https://console.cloud.google.com/cloud-build/builds/07b6a504-71e5-4ab8-b666-628a450ce62f?project=308784392426 ].
----------------------------------------------------------------- REMOTE BUILD OUTPUT -----------------------------------------------------------------
starting build "07b6a504-71e5-4ab8-b666-628a450ce62f"

FETCHSOURCE
Fetching storage object: gs://myproject_cloudbuild/source/1699553041.598345-dc266633efc0492e861e00f570ed15b1.tgz#1699553041917536
Copying gs://myproject_cloudbuild/source/1699553041.598345-dc266633efc0492e861e00f570ed15b1.tgz#1699553041917536...
/ [1 files][  928.0 B/  928.0 B]                                                
Operation completed over 1 objects/928.0 B.
BUILD
Starting Step #0
Step #0: Pulling image: gcr.io/myproject/apptainer
Step #0: Using default tag: latest
Step #0: latest: Pulling from myproject/apptainer

... many more lines ...

Step #0: done.
Step #0: INFO:    Adding environment to container
Step #0: INFO:    Adding runscript
Step #0: INFO:    Creating SIF file...
Step #0: INFO:    Build complete: lolcow.sif
Finished Step #0
Starting Step #1
Step #1: Already have image (with digest): gcr.io/myproject/apptainer
Step #1: INFO:    Upload complete
Finished Step #1
PUSH
DONE
-------------------------------------------------------------------------------------------------------------------------------------------------------
ID                                    CREATE_TIME                DURATION  SOURCE                                                                                           IMAGES  STATUS
07b6a504-71e5-4ab8-b666-628a450ce62f  2023-11-09T18:04:02+00:00  1M4S      gs://myproject_cloudbuild/source/1699553041.598345-dc266633efc0492e861e00f570ed15b1.tgz  -       SUCCESS
```

You can get a list of images in your repository with the commands

```bash
gcloud config set artifacts/location #LOCATION#
gcloud config set artifacts/respository #REPOSITORY_NAME#
gcloud artifacts docker tags list --format="value(IMAGE,TAG)" 2>/dev/null | awk '{ printf "%s:%s\n", $1, $2 }'
```

Where *LOCATION* is the location of your repository, e.g., 'us', and *REPOSITORY_NAME* is the name of the Artifact Registry repository you specified above.

## Deployment

Create an environment variable for your repository URL

```bash
export REPOSITORY_URL=oras://#LOCATION#/#PROJECT_NAME#/#REPOSITORY_NAME# 
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

You retrieve the `lolcow` SIF image you created above using the command

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

You can execute the `lolcow_1.0.sif` file via the `apptainer run` command

```bash
apptainer run lolcow_1.0.sif
```

or just enter `./lolcow_1.0.sif`

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

## Next Steps

Cloud Build and Artifact Registry are components of Google's serverless CI/CD platform. Since SIF definition files and build configurations are regular files you can, and should, manage them using the source control tool chain of your choice. If that tool chain can generate _webhook events_ you can [automate](https://cloud.google.com/build/docs/automate-builds-webhook-events) SIF image builds.