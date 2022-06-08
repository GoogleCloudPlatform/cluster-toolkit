# Google Cloud HPC Toolkit - Application Demos

Click on one of the buttons below to launch a hands on tutorial.

## Gromacs

[![Open in Cloud Shell](https://gstatic.com/cloudssh/images/open-btn.svg)](https://shell.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2FGoogleCloudPlatform%2Fhpc-toolkit&cloudshell_git_branch=application_demo&cloudshell_open_in_editor=community%2Fexamples%2Fworkshops%2Fspack-gromacs.yaml&cloudshell_tutorial=docs%2Ftutorials%2Fgromacs%2Fspack-gromacs.md)

## Openfoam

[![Open in Cloud Shell](https://gstatic.com/cloudssh/images/open-btn.svg)](https://shell.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2FGoogleCloudPlatform%2Fhpc-toolkit&cloudshell_git_branch=application_demo&cloudshell_open_in_editor=community%2Fexamples%2Fworkshops%2Fspack-openfoam.yaml&cloudshell_tutorial=docs%2Ftutorials%2Fopenfoam%2Fspack-openfoam.md)

## Weather Research and Forecasting (WRF) Model

[![Open in Cloud Shell](https://gstatic.com/cloudssh/images/open-btn.svg)](https://shell.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2FGoogleCloudPlatform%2Fhpc-toolkit&cloudshell_git_branch=application_demo&cloudshell_open_in_editor=community%2Fexamples%2Fworkshops%2Fspack-wrfv3.yaml&cloudshell_tutorial=docs%2Ftutorials%2Fwrfv3%2Fspack-wrfv3.md)

## Blueprint Diagram

All the tutorials above use similar blueprints built of from a number of
terraform modules. The diagram below shows how the various modules relate to
each other.

```mermaid
graph TB
    A(Virtual Private Cloud) 
    C(Spack Install Script)
    D(Startup Scripts)
    E(Compute Partition)
    F(Slurm Controller) 
    G(Slurm Login Node)
    B(Monitoring Dashboard)
    C --> D
    A --> E
    A --> F
    E --> F
    D --> F
    A --> G
    F --> G
```
