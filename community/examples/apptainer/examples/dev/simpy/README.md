# Conda in a Container

[Conda](https://docs.conda.io/en/latest/) is a useful tool for installing software with complex dependencies. It has, however, some problems, especially on HPC systems with shared file systems. The main problems of Conda environments are related to storage. Conda environments are quite large, containing tens to hundreds of thousands of files. Moreover, many of these files will be accessed each time you launch a program installed with Conda, generating massive I/O load which may degrade the performance of the system for all users. Conda environments can also be somewhat sensitive to changes in the base system, meaning that, e.g., updates can sometimes break existing Conda environments, necessitating a re-install.

Using an [Apptainer](https://apptainer.org/) container can help with both problems. A container is just a single file that is typically smaller than the total size of the Conda environment directory. It is also less sensitive to changes in the host system.

This example illustrates one way to capture a Conda environment using Apptainer.

In this example you will create an Apptainer container that packages a conda installation of the [SimPy](https://simpy.readthedocs.io/en/latest/) discrete event simulator. You will build the container using [Google Cloud Build](https://cloud.google.com/build?hl=en), store it in [Google Artifact Registry](https://cloud.google.com/artifact-registry), and then deploy and run it in a [Slurm](https://slurm.schedmd.com/documentation.html)-based HPC System deployed with the [Cloud HPC Toolkit](https://cloud.google.com/hpc-toolkit/docs/overview).

### Before you begin
This demonstration assumes you have access to an [Artifact Registry](https://cloud.google.com/artifact-registry) repository and that you have set up the Apptainer custom build step. See [this section](../../../README.md#before-you-begin) for details.

## Container Definition

The [simpy.def](./simpy.def) file defines the construction of the container by the `apptainer build` command. `simpy.def` defines a [mult-stage build](https://apptainer.org/docs/user/latest/definition_files.html#multi-stage-builds) that separates the container construction into `install` and `runtime` stages. While not strictly necessary for this simple example it is a best practice which often creates a smaller final image without the entire development stack.

The Conda environment is the key to capturing all of the package dependencies and configuration files in the container. In the `%post` section of the `install` stage a conda environment is created, `simpy` installed and the resulting environment is exported.

```
%post
    . /opt/conda/etc/profile.d/conda.sh
    conda create -y -n simpy python=3.11
    conda activate simpy
    conda install -y -c conda-forge simpy
    conda env export > /usr/local/share/environment.yaml
    conda deactivate
```

In the `runtime` stage the `environment.yaml` file generated in the `install` stage is copied in, everything else that was installed and/or created in the `install` stage is discarded.

```
%files from install
    /usr/local/share/environment.yaml /usr/local/share/environment.yaml
```

The first three lines of the `runtime` `%post` section use the [APPTAINER_ENVIRONMENT](https://apptainer.org/docs/user/latest/environment_and_metadata.html#build-time-variables-in-post) mechanism to generate a shell script that is sources when the container is executed. 

The remaining three lines of the `runtime` `%post` section create a conda environment from the `environment.yaml` file copied from the `install` stage.

```
%post
    ENV_NAME=$(head -1 /usr/local/share/environment.yaml | cut -d' ' -f2)
    echo ". /opt/conda/etc/profile.d/conda.sh" >> $APPTAINER_ENVIRONMENT
    echo "source activate $ENV_NAME" >> $APPTAINER_ENVIRONMENT

    . /opt/conda/etc/profile.d/conda.sh
    conda env create -f /usr/local/share/environment.yaml -p /opt/conda/envs/$ENV_NAME
    conda clean --all
```

Finally the `%runscript` section defines what happens when the container is executed.

```
%runscript
    exec "$@"
```
## Container Build

You build the `simpy` container and save it to Artifact Registry with the command

```bash
gcloud builds submit --config=simpybuild.yaml
```

Note that this will used the default values for
- _LOCATION: _*us-docker.pkg.dev*_
- _REPOSITORY: _*sifs*_
- _VERSION: _*latest*_

If you want to change any of these values add the `--substitution` switch to the command above, e.g., to set the version to `1.0`

```bash
gcloud builds submit --config=simpybuild.yaml --substitutions=_VERSION=1.0
```

## Usage

To use the `simpy` container you built, deploy a Slurm-based HPC System using the [slurm-apptainer.yaml](../../../cluster/slurm-apptainer.yaml) blueprint following the process described [here](../../../cluster/README.md). Login to the HPC system's login node with the command

```bash
gcloud compute ssh \
  $(gcloud compute instances list \
      --filter="NAME ~ login" \
      --format="value(NAME)") \
  --tunnel-through-iap
```

Create a simulation using the command

```python
cat <<- "EOF" > bank_revenge.py
"""
Bank renege example

Covers:

- Resources: Resource
- Condition events

Scenario:
  A counter with a random service time and customers who renege. Based on the
  program bank08.py from TheBank tutorial of SimPy 2. (KGM)

"""
import random

import simpy

RANDOM_SEED = 42
NEW_CUSTOMERS = 5  # Total number of customers
INTERVAL_CUSTOMERS = 10.0  # Generate new customers roughly every x seconds
MIN_PATIENCE = 1  # Min. customer patience
MAX_PATIENCE = 3  # Max. customer patience


def source(env, number, interval, counter):
    """Source generates customers randomly"""
    for i in range(number):
        c = customer(env, f'Customer{i:02d}', counter, time_in_bank=12.0)
        env.process(c)
        t = random.expovariate(1.0 / interval)
        yield env.timeout(t)


def customer(env, name, counter, time_in_bank):
    """Customer arrives, is served and leaves."""
    arrive = env.now
    print(f'{arrive:7.4f} {name}: Here I am')

    with counter.request() as req:
        patience = random.uniform(MIN_PATIENCE, MAX_PATIENCE)
        # Wait for the counter or abort at the end of our tether
        results = yield req | env.timeout(patience)

        wait = env.now - arrive

        if req in results:
            # We got to the counter
            print(f'{env.now:7.4f} {name}: Waited {wait:6.3f}')

            tib = random.expovariate(1.0 / time_in_bank)
            yield env.timeout(tib)
            print(f'{env.now:7.4f} {name}: Finished')

        else:
            # We reneged
            print(f'{env.now:7.4f} {name}: RENEGED after {wait:6.3f}')


# Setup and start the simulation
print('Bank renege')
random.seed(RANDOM_SEED)
env = simpy.Environment()

# Start processes and run
counter = simpy.Resource(env, capacity=1)
env.process(source(env, NEW_CUSTOMERS, INTERVAL_CUSTOMERS, counter))
env.run()
EOF
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

Then run the simulation on a compute node

```bash
srun -N1 apptainer run $REPOSITORY_URL/simpy:1.0 python3 bank_revenge.py
```

You should see output that looks like

```
Bank renege
 0.0000 Customer00: Here I am
 0.0000 Customer00: Waited  0.000
 3.8595 Customer00: Finished
10.2006 Customer01: Here I am
10.2006 Customer01: Waited  0.000
12.7265 Customer02: Here I am
13.9003 Customer02: RENEGED after  1.174
23.7507 Customer01: Finished
34.9993 Customer03: Here I am
34.9993 Customer03: Waited  0.000
37.9599 Customer03: Finished
40.4798 Customer04: Here I am
40.4798 Customer04: Waited  0.000
43.1401 Customer04: Finished
```

## References

This example drew heavily from work at
- The [CSC Computing Environent](https://csc-training.github.io/csc-env-eff/hands-on/singularity/singularity_extra_replicating-conda.html)
- [NHR@Göttingen](https://gitlab-ce.gwdg.de/hpc-team-public/science-domains-blog/-/blob/main/20230907_python-apptainer.md)

For a general discussion of containers in an HPC system see [this](https://github.com/dirkpetersen/hpc-containers#why-this-article-)
