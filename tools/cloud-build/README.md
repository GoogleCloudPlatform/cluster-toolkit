# Cloud Build Tools

## Contents

* `daily-tests`: The daily-tests directory contains cloud build configs and
  support files for running the daily test suite
* `dependency-checks`: Verifies the `gcluster` build in limited dependency
  environments.
* `ansible.cfg`: Ansible config used to set common ansible setting for running
  the test suite.
* `hpc-toolkit-pr-validation.yaml`: Cloud build config for the PR validation
  tests. The PR validation run `make tests` and validates against all
  pre-commits on all files.
* `pr-ofe.yaml`: Cloud build config for sanity test installing the OFE virtual environment.
* `project-cleanup.yaml`: Cloud build config that performs a regular cleanup of
  resources in the test project.
* `provision`: Terraform module that sets up CloudBuild triggers and schedule.

## Kueue Lock Automation

When creating a new test, you must request a test lock in your job definition (e.g., `test-locks/<test-name>: 1`).

The pipeline uses `pre-commit` hooks to enforce and automate test lock registration:

1. `validate_kueue_tests.py`: Ensures your test job requests a lock and auto-fixes the `cpu` and `memory` limits/requests if they are missing or non-standard.
2. `generate_kueue_locks.py`: Automatically detects new locks requested in build files and appends them to the Kueue configurations in `daily-tests/blueprints/test-infra-kueue/configs/`.

If the `generate-kueue-locks` hook modifies files during your commit, simply review the changes and run `git add` again.

Newly generated locks are automatically applied to the test clusters in the CI pipeline whenever `terraform apply` is run in the `provision` module.

### Local Testing with Kueue

If you wish to test your new test build locally against the test clusters before pushing your code, you must first generate the Kueue configurations and then manually apply them. You can do this by running the following from the root of the repository:

```bash
pre-commit run validate-kueue-tests --all-files
pre-commit run generate-kueue-locks --all-files

cd tools/cloud-build/provision
./apply_kueue_locks.sh
```
