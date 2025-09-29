# DWS Calendar Consumption Option

[Dynamic Workload Scheduler (DWS)](https://cloud.google.com/blog/products/compute/introducing-dynamic-workload-scheduler) is a resource management and job scheduling platform designed for AI Hypercomputer. Dynamic Workload Scheduler improves your access to AI/ML resources, helps you optimize your spend, and can improve the experience of workloads such as training and fine-tuning jobs, by scheduling all the accelerators needed simultaneously. Dynamic Workload Scheduler supports TPUs and NVIDIA GPUs, and brings scheduling advancements from Google ML fleet to Google Cloud customers.

With Calendar mode, you will be able to request GPU capacity in fixed duration capacity blocks. Additional information about DWS Calendar mode can be [found here](https://cloud.google.com/blog/products/compute/introducing-dynamic-workload-scheduler).

## Create an A3 Ultra cluster

The [gke-a3-ultragpu](./examples/gke-a3-ultragpu) example can be used to create an A3 Ultra cluster using a DWS Calendar reservation. The `reservation` variable under `vars` in the [-deployment.yaml](examples/gke-a3-ultragpu/gke-a3-ultragpu-deployment.yaml) should be updated to the DWS Calendar reservation name.

Refer to [Create an AI-optimized GKE cluster with default configuration](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#use-cluster-toolkit) for instructions on creating the GKE-A3U cluster.

Refer to [Deploy and run NCCL test](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#deploy-run-nccl-tas-test) for instructions on running a NCCL test on the GKE A3 Ultragpu cluster.
