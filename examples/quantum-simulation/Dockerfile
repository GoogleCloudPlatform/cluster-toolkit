# Start from the cuquantum appliance image
FROM nvcr.io/nvidia/cuquantum-appliance:24.08-x86_64

# Update CUDA 
RUN conda update -y --all && conda install -y cuda-cudart cuda-version=12

# Install additional dependencies
# RUN conda init bash && conda install fontconfig && /opt/conda/envs/cuquantum-24.08/bin/python3.11 -m pip install nvtx psutil scipy networkx

# Install latest cuQuantum
WORKDIR /opt
RUN sudo chmod 1777 /opt
RUN git clone https://github.com/NVIDIA/cuQuantum.git
RUN cd /opt/cuQuantum/benchmarks/ && /opt/conda/envs/cuquantum-24.08/bin/pip install .[all]

# Set environment variables
ENV LD_LIBRARY_PATH=/usr/lib/x86_64-gnu:/opt/conda/envs/cuquantum-24.08/lib:/usr/local/mpi/lib:/usr/local/ucx/lib:/usr/local/gdrcopy/lib:/usr/local/munge/lib:/usr/local/pmix/lib:/usr/local/slurm/lib:/usr/local/nvidia/lib:/usr/local/cuda/lib64:/usr/local/cuda/targets/x86_64-linux/lib:${LD_LIBRARY_PATH}
ENV CUDA_VISIBLE_DEVICE=0,1,2,3,4,5,6,7
ENV MPLCONFIGDIR=/opt/.matplotlib
ENV NVSHMEM_DISABLE_CUDA_VMM=1

# MPI Environment variables
ENV OMPI_MCA_btl=^openib
ENV OMPI_MCA_btl_tcp_if_include=enp0s19
ENV OMPI_MCA_coll=^hcoll
ENV OMPI_MCA_coll_hcoll_enable=0
ENV OMPI_MCA_opal_cuda_support=true
ENV OMPI_MCA_pml=ucx
ENV PMIX_MCA_gds=hash

# Set UCX Environment Variables
ENV UCX_IB_GID_INDEX=3 
ENV UCX_IB_GPU_DIRECT_RDMA=1
ENV UCX_IB_PCI_RELAXED_ORDERING=on
ENV UCX_IB_SL=2
ENV UCX_MAX_RNDV_RAILS=2
ENV UCX_MEMTYPE_CACHE=n
ENV UCX_NET_DEVICES=mlx5_0:1,mlx5_1:1,mlx5_2:1,mlx5_3:1,mlx5_4:1,mlx5_5:1,mlx5_6:1,mlx5_7:1
ENV UCX_RC_VERBS_SL=2;
ENV UCX_RNDV_SCHEME=get_zcopy
ENV UCX_RNDV_THRESH=8192
ENV UCX_TLS=all
