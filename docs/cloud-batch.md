# Google Cloud Batch in the HPC Toolkit

Using Google Cloud Batch with the HPC Toolkit simplifies the setup needed to
provision and run more complex scenarios, for example, setting up a shared file
system and installing software to be used by Google Cloud Batch jobs. It also
makes it possible to share tested infrastructure solutions that work with Google
Cloud Batch via HPC Toolkit blueprints.

## Modules

The HPC Toolkit supports Google Cloud Batch through two Toolkit modules:

- [batch-job-template](../modules/scheduler/batch-job-template/README.md):
  - Generates a Google Cloud Batch job template that can be submitted to the
    Google Cloud Batch API
  - Creates an instance template for the Google Cloud Batch job to use
  - Works with existing Toolkit modules such as `vpc`, `filestore`,
    `startup-script` & `spack`
- [batch-login-node](../modules/scheduler/batch-login-node/README.md)
  - Creates a login node VM for Google Cloud Batch job submission

See links above for additional documentation on each module. These modules are
contained in the `community` folder of the HPC Toolkit repo and are marked as
`experimental` while Google Cloud Batch is in public preview.

## Example

[cloud-batch.yaml](../examples/cloud-batch.yaml) contains an example
of how to use Google Cloud Batch with the HPC Toolkit
([example documentation](../examples/README.md#cloud-batchyaml--)).

---

For general information on using the HPC Toolkit see
[this quickstart documentation](../README.md#quickstart).
