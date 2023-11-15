# Containerizing Development Environments

You can use Apptainer to package your development environment to streamline your workflow in a cluster deployed via the HPC Toolkit. We provide examples of 
- [simpy](./simpy/README.md) which packages a [miniconda](https://docs.conda.io/projects/miniconda/en/latest/) environment using Apptainer and then deploying and using it in a Slurm allocation
- [vscode](./vscode/README.md) which packages the [VS Code](https://code.visualstudio.com/) IDE, deploying it in a Slurm allocation and connecting to if from your local VS Code IDE