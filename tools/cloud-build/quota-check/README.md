# `quota-check` tool

`quota-check` is a tool to verify that GCP project has enough quota across multiple regions and zones.

## Usage

* Configure desired amount of resource quotas in `bp.yaml`;
* Configure set of regions and zones in `check.py`;
* Run the tool:

```sh
tools/cloud-build/quota-check/check.py --project=<MY_PROJECT>
```
