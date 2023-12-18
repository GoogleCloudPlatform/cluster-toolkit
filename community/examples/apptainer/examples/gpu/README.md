# GPUs and Apptainer

Many modern HPC codes take advantage of the massively parallel execution capability GPUs afford. Apptainer provides seamless integration with NVIDIA devices on Google Cloud. This example illustrates how code packaged by Apptainer can take advantage of GPU devices attached to a compute host.

The [Julia](https://julialang.org/) programming language provides a seamless interface to [NVIDIA](https://juliagpu.org/cuda/) and [AMD](https://juliagpu.org/rocm/) GPUs. Google supports a broad range of [NVIDIA GPUs](https://cloud.google.com/gpu). The [Cloud HPC Toolkit](https://cloud.google.com/hpc-toolkit/docs/overview) creates Slurm-based HPC systems whose compute nodes have the NVIDIA CUDA drivers and runtime installed and can be configured with one or more GPUs attached. You will use Apptainers built-in support for NVIDIA GPUs to deploy as simple massively parallel code: [SAXPY](https://developer.nvidia.com/blog/n-ways-to-saxpy-demonstrating-the-breadth-of-gpu-programming-options/) the `hello world` of massively parallel programming.

### Before you begin
This demonstration assumes you have access to an [Artifact Registry](https://cloud.google.com/artifact-registry) repository and that you have set up the Apptainer custom build step. See [this section](../../README.md#before-you-begin) for details.

## Setup

Deploy a Slurm-based HPC system using the [slurm-apptainer-gpu.yaml](../../../cluster/slurm-apptainer-gpu.yaml) blueprint following the process described [here](../../../cluster/README.md). Login to the HPC system's login node with the command

```bash
gcloud compute ssh \
  $(gcloud compute instances list \
      --filter="NAME ~ login" \
      --format="value(NAME)") \
  --tunnel-through-iap
```

This example takes advantage of Apptainers ability to [transform](https://apptainer.org/docs/user/latest/build_a_container.html#converting-containers-from-one-format-to-another) [OCI](https://opencontainers.org/) container images into [SIF](https://github.com/apptainer/sif) images. Rather than building a separate `Julia` SIF image you can just use the version [publicly available](https://hub.docker.com/_/julia) in [Docker Hub](https://hub.docker.com/). You will, however, need to use the `Julia` package manager to add the [CUDA](https://github.com/JuliaGPU/CUDA.jl) and [BenchmarkTools](https://juliaci.github.io/BenchmarkTools.jl/stable/) packages. By default SIF images are readonly, but you can specify a writeable [persistent overlay](https://apptainer.org/docs/user/latest/persistent_overlays.html) at runtime. You will use this mechanism to add the necessary `Julia` packages.

Create the persistent overlay with the command

```bash
apptainer overlay create --sparse --size 8192 --create-dir /usr/local/share/applications/julia/depot --fakeroot ./julia_depot.img
```

The `--sparse` switch directs `apptainer` to create a [sparse](https://apptainer.org/docs/user/latest/persistent_overlays.html#sparse-overlay-images) overlay which on takes up space on disk as data is written to it. The `--create-dir` switch creates a directory in the overlay owned by the calling user. This particular directory is where the `Julia` packages you install will be stored. Finally, the `--fakeroot` switch is added to allow you to modify a container with resulting overlay.

## Juila Packages

When `Julia` adds packages it downloads them to a directory designated by the _JULIA_DEPOT_PATH_ environment variable. For this example you will use the

```/usr/local/share/applications/juila/depot```

directory that the persistent overlay you just created contains.

Use these commands to set the _JUILIA_DEPOT_PATH_ environment variable and then add the packages to the persistent overlay

```bash
export JULIA_DEPOT_PATH=/usr/local/share/applications/julia/depot:$JULIA_DEPOT_PATH
apptainer exec --overlay ./julia_depot.img --fakeroot docker://julia julia -e 'using Pkg; Pkg.add("CUDA"); Pkg.add("BenchmarkTools")'
```

Downloading and pre-compiling the `CUDA` and `BenchmarkTools` packages and all their supporting packages will several minutes. When the process is complete you will see output that looks like

```
...

  63 dependencies successfully precompiled in 248 seconds. 5 already precompiled.
   Resolving package versions...
   Installed JSON ─────────── v0.21.4
   Installed BenchmarkTools ─ v1.3.2
    Updating `/usr/local/share/applications/julia/newdepot/environments/v1.9/Project.toml`
  [6e4b80f9] + BenchmarkTools v1.3.2
    Updating `/usr/local/share/applications/julia/newdepot/environments/v1.9/Manifest.toml`
  [6e4b80f9] + BenchmarkTools v1.3.2
  [682c06a0] + JSON v0.21.4
  [a63ad114] + Mmap
  [9abbd945] + Profile
Precompiling project...
  2 dependencies successfully precompiled in 4 seconds. 68 already precompiled
```

## Verify Environment

To verify that your environment is configured properly, get a node allocation with the command

```bash
salloc -p gpu -N1 --gpus-per-node=1
```

On the compute node execute

```bash
nvidia-smi
```

You should see output simillar to

```
Mon Nov 20 17:20:30 2023       
+---------------------------------------------------------------------------------------+
| NVIDIA-SMI 535.86.10              Driver Version: 535.86.10    CUDA Version: 12.2     |
|-----------------------------------------+----------------------+----------------------+
| GPU  Name                 Persistence-M | Bus-Id        Disp.A | Volatile Uncorr. ECC |
| Fan  Temp   Perf          Pwr:Usage/Cap |         Memory-Usage | GPU-Util  Compute M. |
|                                         |                      |               MIG M. |
|=========================================+======================+======================|
|   0  Tesla V100-SXM2-16GB           On  | 00000000:00:04.0 Off |                    0 |
| N/A   37C    P0              26W / 300W |      0MiB / 16384MiB |      0%      Default |
|                                         |                      |                  N/A |
+-----------------------------------------+----------------------+----------------------+
                                                                                         
+---------------------------------------------------------------------------------------+
| Processes:                                                                            |
|  GPU   GI   CI        PID   Type   Process name                            GPU Memory |
|        ID   ID                                                             Usage      |
|=======================================================================================|
|  No running processes found                                                           |
+---------------------------------------------------------------------------------------+
```

Now you will make sure that the `Julia` CUDA package is setup correctly. Make sure the _JULIA_DEPOT_PATH_ is set correctly

```bash
echo $JULIA_DEPOT_PATH
```

The resoponse should be

```
usr/local/share/applications/julia/depot
``` 

Then start the `Julia` REPL with the command

```bash
apptainer run --nv --overlay ./julia_depot.img --fakeroot docker://julia
```

The `Julia` REPL looks like

```
   _       _ _(_)_     |  Documentation: https://docs.julialang.org
  (_)     | (_) (_)    |
   _ _   _| |_  __ _   |  Type "?" for help, "]?" for Pkg help.
  | | | | | | |/ _` |  |
  | | |_| | | | (_| |  |  Version 1.9.4 (2023-11-14)
 _/ |\__'_|_|_|\__'_|  |  Official https://julialang.org/ release
|__/                   |
```

Now check to see if CUDA is setup properly

```julia
using CUDA
CUDA.functional()
```

The response should be `true`

```julia
CUDA.version_info()
```

The response should be similar to

```
CUDA runtime 12.3, artifact installation
CUDA driver 12.3
NVIDIA driver 535.86.10, originally for CUDA 12.2

CUDA libraries: 
- CUBLAS: 12.3.2
- CURAND: 10.3.4
- CUFFT: 11.0.11
- CUSOLVER: 11.5.3
- CUSPARSE: 12.1.3
- CUPTI: 21.0.0
- NVML: 12.0.0+535.86.10

Julia packages: 
- CUDA: 5.1.0
- CUDA_Driver_jll: 0.7.0+0
- CUDA_Runtime_jll: 0.10.0+1

Toolchain:
- Julia: 1.9.4
- LLVM: 14.0.6

1 device:
  0: Tesla V100-SXM2-16GB (sm_70, 15.770 GiB / 16.000 GiB available)
```

If there is a problem the error messages should give you a clue as to source of the problem.

If the environment is configured correctly exit the `Julia` REPL with `^d` and leave the allocation with another `^d`.

## SAXPY

Run the following command to create a local version of a simple SAXPY program that you will use to run CPU and GPU benchmarks

```bash
cat <<- "EOF" > saxpy.jl
# Compute SAXPY (single-precision a*x + y for scalar a and vectors x and y)
# Use Julia broadcasting first on the CPU then with the GPU
#
using CUDA, BenchmarkTools, Printf

dim = 100_000_000
a = 3.1415

# CPU variables
x = ones(Float32, dim)
y = ones(Float32, dim)
z = zeros(Float32, dim)

# Force compiliation to get a better benchmark
z .= a .* x .+ y;

b= @benchmark z .= a .* y .+ x
@printf "CPU Median: %s\n" median(b)

# GPU variables
x_d = CUDA.ones(Float32, dim)
y_d = CUDA.ones(Float32, dim)
z_d = CUDA.zeros(Float32, dim)

# Force compilation to get a better benchmark
CUDA.@sync z_d .= a .* y_d .+ x_d;

b = @benchmark CUDA.@sync z_d .= a .* y_d .+ x_d
@printf "GPU Median: %s\n" median(b)
EOF
```

To run saxpy.jl on a compute node execute the command

```bash
srun -p gpu -N1 --gpus-per-node=1 apptainer run --nv --overlay ./julia_depot.img --fakeroot docker://julia ./saxpy.jl
```

The output should be similar to

```
INFO:    Using cached SIF image
INFO:    User not listed in /etc/subuid, trying root-mapped namespace
INFO:    Using cached SIF image
INFO:    Using fakeroot command combined with root-mapped namespace
INFO:    unknown argument ignored: lazytime
CPU Median: TrialEstimate(105.737 ms)
GPU Median: TrialEstimate(1.638 ms)
```

This particular code, which is massively parallel and scales with the size of the problem, is about two orders of magnitude faster on the GPU.