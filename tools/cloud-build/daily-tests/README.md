# Daily and integration tests for the toolkit

Integration tests have been broken into multiple steps. This allows easily
adding new integration tests as build steps under hpc-toolkit-integration-tests.

Cloud build calls ansible-playbook
`[slurm-integration-tests | basic-integration-tests]` with a custom
configuration yaml. Each test has its own yaml under
tools/cloud-build/daily-tests/tests. This file specifies common variables and a
list of post_deploy_test, which can be an empty array for tests that only
validate deployment. Or can list various extra tasks (only one implemented now:
`test-mounts-and-partitions`). This file also specifies the blueprint to create
the HPC environment

The integration test yml, either `slurm-integration-tests` or
`basic-integration-tests`, under ansible_playbooks, in turn calls the creation
of the blueprint (create_deployment.sh) and the post_deploy_tests.

To run the tests on your own project, with your own files, use:

```shell
gcloud builds submit --config tools/cloud-build/daily-tests/hpc-toolkit-integration-tests.yaml
```
