# Cloud Build Tools

## Contents

* `daily-tests`: The daily-tests directory contains cloud build configs and
  support files for running the daily test suite
* `dependency-checks`: Verifies the `ghpc` build in limited dependency
  environments.
* `ansible.cfg`: Ansible config used to set common ansible setting for running
  the test suite.
* `Dockerfile`: Defines the HPC Toolkit docker image used in testing.
* `hpc-toolkit-builder.yaml`: Cloud build config for running regular builds of
  the HPC Toolkit docker image.
* `hpc-toolkit-pr-validation.yaml`: Cloud build config for the PR validation
  tests. The PR validation run `make tests` and validates against all
  pre-commits on all files.
* `pr-ofe.yaml`: Cloud build config for sanity test installing the OFE virtual environment.
* `project-cleanup.yaml`: Cloud build config that performs a regular cleanup of
  resources in the test project.
* `provision`: Terraform module that sets up CloudBuild triggers and schedule.
