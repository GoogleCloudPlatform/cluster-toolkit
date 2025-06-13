# Simple Ipynb Launcher

The Simple Ipynb Launcher is a Jupyter Notebook-based interface designed to streamline the process of running AlphaFold 3 jobs through a SLURM-based cluster using the SLURM REST API.

It allows users to:

- Upload AlphaFold 3 input files (see [AlphaFold 3 Input Documentation](https://github.com/google-deepmind/alphafold3/blob/main/docs/input.md)),
- Launch data pipeline and/or inference jobs, and
- View and validate output files — all within a Jupyter Notebook environment.

## 📦 1. Setup Guide: Jupyter Notebook with SLURM

This setup guide explains how to configure and launch the Simple IPython Launcher environment, which enables interactive AlphaFold 3 workflows via Jupyter Notebook and SLURM REST API.

There are **two setup stages** depending on whether your SLURM cluster is already deployed:

- **[Pre-Cluster Deployment](./Setup-pre-cluster-deployment.md)**:
   Ipynb notebook deployment steps **before deploying** the `af3-slurm.yaml` SLURM cluster.

- **[Post-Cluster Deployment](./Setup-post-cluster-deployment.md)**:  
   Ipynb notebook deployment steps **after deploying** the `af3-slurm.yaml` SLURM cluster.

## ▶️ 2. [Usage Guide: Step-by-Step Instructions](./Ipynb.md)

After the Ipynb Launcher has been successfully launched, refer to this usage guide to:

- Follow a step-by-step walkthrough of each section  
- Learn how to input data, run data pipeline and inference  
- Visualizing the Results
