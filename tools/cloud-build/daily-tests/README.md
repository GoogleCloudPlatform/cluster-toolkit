# Integration tests for the toolkit

Cloud build calls ansible-playbook `*-integration-tests.yml` with a custom
configuration yaml. Each test has its own yaml under
tools/cloud-build/daily-tests/tests. This file specifies common variables and a
list of post_deploy_test, which can be an empty array for tests that only
validate deployment. Or can list various extra tasks, named `test-*.yml. This
file also specifies the blueprint to create the AI/ML and HPC environment

The integration test yml under ansible_playbooks, in turn calls the creation of
the blueprint (create_deployment.sh) and the post_deploy_tests.

To run the tests on your own project, with your own files, use:

```shell
gcloud builds submit --config tools/cloud-build/daily-tests/builds/test_name.yaml
```

Note: If you are testing `ofe-deployment-integration-test` locally,
you need to first disable gcloudignore.

```shell
gcloud config set gcloudignore/enabled false
```

## Hello World Integration Test

The hello world integration test exists to demonstrate the test interaction
between test files, and can be used to test passing variables without having to
actually run integration test on cloud build.

This example consists of 3 files:

- tools/cloud-build/daily-tests/ansible_playbooks/hello-world-integration-test.yml
  - The playbook that is the root of the test
- tools/cloud-build/daily-tests/ansible_playbooks/test-validation/test-hello-world.yml
  - The post deploy test (tasks) that is called by the playbook
- tools/cloud-build/daily-tests/tests/hello-world-vars.yml
  - The variables passed into the playbook
