README
======

1. Set up NeMo Framework Container

   This makes a few environment variable modifications to the [nvcr.io/nvidia/nemo:24.07](https://catalog.ngc.nvidia.com/orgs/nvidia/containers/nemo)
   container, and submits a Slurm job to copy the framework launcher scripts and a
   few other auxiliary files into your working directory.

   ```shell
   sbatch setup_nemo.sh
   ```

2. Install NeMo Framework Requirements

   We suggest using a virtual environment, and this installs the necessary
   components to submit jobs using the NeMo
   framework.

   ```shell
   python3 -m venv env
   source env/bin/activate
   pip install -r requirements.txt # Copied from the NeMo Framework Container earlier
   # This is needed to use 24.07 and python3.11, which is what is present on
   # Debian 12
   pip install -U hydra-core
   ```

3. Run an example NeMo Framework Pre-Training

   First, prepare the cache. This will download several files to the
   ~/.cache/huggingface folder which are needed to load the tokenizer for
   training.

   ```shell
   pip install transformers
   python -c "from transformers import AutoTokenizer; \
       AutoTokenizer.from_pretrained('gpt2')"
   ```

   This will run an example of training a 5B parameter GPT3 model for 10 steps
   using mock data as the input.

   ```shell
   cd launcher_scripts
   mkdir data

   MAX_STEPS=10
   NUM_NODES=8

   python main.py \
       launcher_scripts_path=${PWD} \
       stages=[training] \
       training=gpt3/5b \
       env_vars.TRANSFORMERS_OFFLINE=0 \
       container=../nemofw+tcpxo-24.07.sqsh \
       container_mounts=[${HOME}/.cache,/var/lib/tcpxo/lib64] \
       cluster.srun_args=["--container-writable"] \
       training.model.data.data_impl=mock \
       training.model.data.data_prefix=[] \
       training.trainer.max_steps=${MAX_STEPS} \
       training.trainer.val_check_interval=${MAX_STEPS} \
       training.trainer.limit_val_batches=0.0 \
       training.exp_manager.create_checkpoint_callback=False \
       training.exp_manager.resume_if_exists=False \
       training.trainer.num_nodes=${NUM_NODES}
   ```

   This will submit a pre-training job to your Slurm cluster. Once it starts, you
   will see results appearing in `results/gpt3_5b/`. For this example, the job
   should only take a few minutes.

Next Steps
----------

Now that you've run an example training workload, you may find it preferable to
customize conf/cluster/bcm.yaml, conf/config.yaml, and the training
configuration file of your choosing as opposed to using command line arguments.
For real training workloads you'll also want to use real data, as opposed to
the mock datasets used here, and explore all tuning and configurations
parameters for your use case through the NeMo Framework.
