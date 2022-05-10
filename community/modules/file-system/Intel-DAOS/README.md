## Description

This modules allows creating an instance of Distributed Asynchronous Object Storage ([DAOS](https://docs.daos.io/)) on Google Cloud Platform ([GCP](https://cloud.google.com/)).

For more information, please refer to the [Google Cloud DAOS repo on GitHub](https://github.com/daos-stack/google-cloud-daos).

### Example

Multiple fully working examples of a DAOS deployment and how it can be used in conjunction with Slurm [can be found in the community examples folder](../../../examples/intel/).

Using the DAOS server implies that one has DAOS server images created as [instructed in the images section here](https://github.com/daos-stack/google-cloud-daos/tree/main/images).

A full list of module parameters can be found at [the DAOS Server module README](https://github.com/daos-stack/google-cloud-daos/tree/main/terraform/modules/daos_server).

```yaml
  - source: github.com/daos-stack/google-cloud-daos.git//terraform/modules/daos_server?ref=d1d0f60
    kind: terraform
    id: daos
    use: [network1]
    settings:
    # The following line allows us to run clients without certificates, which is needed for now.
      allow_insecure: true
      pools:
      - pool_name: "test_pool"
        pool_size: "1TB"
        containers:
        - "test_container"
```
