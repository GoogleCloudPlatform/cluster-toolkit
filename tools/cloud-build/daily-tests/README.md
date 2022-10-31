# Daily and integration tests for the toolkit

Integration tests have been broken into multiple steps. This allows easily
adding new integration tests as build steps under hpc-toolkit-integration-tests.

Cloud build calls ansible-playbook `*-integration-tests.yml` with a custom
configuration yaml. Each test has its own yaml under
tools/cloud-build/daily-tests/tests. This file specifies common variables and a
list of post_deploy_test, which can be an empty array for tests that only
validate deployment. Or can list various extra tasks, named `test-*.yml. This
file also specifies the blueprint to create the HPC environment

The integration test yml under ansible_playbooks, in turn calls the creation of
the blueprint (create_deployment.sh) and the post_deploy_tests.

To run the tests on your own project, with your own files, use:

```shell
gcloud builds submit --config tools/cloud-build/daily-tests/integration-group-1.yaml
```

## Hello World Integration Test

The hello world integration test exists to demonstrate the test interaction
between test files, and can be used to test passing variables without having to
actually run integration test on cloud build.

This example consists of 3 files:

- tools/cloud-build/daily-tests/ansible_playbooks/hello-world-integration-test.yml
  - The playbook that is the root of the test
- tools/cloud-build/daily-tests/ansible_playbooks/test-hello-world.yml
  - The post deploy test (tasks) that is called by the playbook
- tools/cloud-build/daily-tests/tests/hello-world-vars.yml
  - The variables passed into the playbook

## Nightly Test Groups

Each test group can be run concurrently, and some tests within test groups run
currently with each other as well. Each group begins with 2 steps for
initializing the environment:

- `build_ghpc`: Build `ghpc` in a golang builder container, verifying that no
  additional dependencies are required for a simple build.
- `fetch_builder`: Simply pulls the `hpc-toolkit-builder` image to the local
  environment to decrease the overhead in subsequent steps.

The tests in each group are listed below:

### Group 1

Config: [integration-group-1.yaml](./integration-group-1.yaml)

Contents:

- `hpc-high-io`: Tests the [hpc-cluster-high-io.yaml] example blueprint.

[hpc-cluster-high-io.yaml]: ../../../examples/hpc-cluster-high-io.yaml

### Group 2

Config: [integration-group-2.yaml](./integration-group-2.yaml)

Contents:

- `spack-gromacs`: Tests the [spack-gromacs.yaml] example blueprint.
- `slurm-gcp-v5-hpc-centos7`: Tests the [slurm-gcp-v5-hpc-centos7.yaml] example
  blueprint.

Notes:

- This test pulls a secret from the hpc-toolkit-dev project which contains the
  path to the internal Spack build cache.

[spack-gromacs.yaml]: ../../../community/examples/spack-gromacs.yaml
[slurm-gcp-v5-hpc-centos7.yaml]: ../../../community/examples/slurm-gcp-v5-hpc-centos7.yaml

### Group 3

Config: [integration-group-3.yaml](./integration-group-3.yaml)

Contents:

- `monitoring`: Tests a blueprint with a monitoring dashboard.
- `omnia`: Tests the [omnia-cluster.yaml] example blueprint.
- `lustre-new-vpc`: Tests the `lustre-with-new-vpc.yaml` blueprint, which
  includes testing DDN lustre with a newly created VPC network as well as other
  unique configurations not tested elsewhere.
- `packer`: Tests the [image-builder.yaml] example blueprint.
- `quantum-circuit`: Tests the [quantum-circuit-simulator.yaml] example
  blueprint.

[omnia-cluster.yaml]: ../../../community/examples/omnia-cluster.yaml
[image-builder.yaml]: ../../../examples/image-builder.yaml
[quantum-circuit-simulator.yaml]: ../../../community/examples/quantum-circuit-simulator.yaml

### Group 4

Config: [integration-group-4.yaml](./integration-group-4.yaml)

Contents:

- `htcondor`: Tests the [htcondor-pool.yaml] example blueprint.
- `cloud-batch`: Tests the [cloud-batch.yaml] example blueprint.
- `slurm-gcp-v5-ubuntu`: Tests the [slurm-gcp-v5-ubuntu2004.yaml] example blueprint.

[htcondor-pool.yaml]: ../../../community/examples/htcondor-pool.yaml
[cloud-batch.yaml]: ../../../examples/cloud-batch.yaml
[slurm-gcp-v5-ubuntu2004.yaml]: ../../../community/examples/slurm-gcp-v5-ubuntu2004.yaml
