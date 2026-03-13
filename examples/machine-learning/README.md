# AI / ML Toolkit Blueprints

The directories below contain solutions for provisioning Slurm clusters using
the latest VM families from Google Cloud

- [A3 High](a3-highgpu-8g)
- [A3 Mega](a3-megagpu-8g)
- [A3 Ultra](a3-ultragpu-8g)
- [A4](a4-highgpu-8g)
- [A4X](a4x-highgpu-4g)

Further documentation for A3 Ultra, A4 and A4X solutions are available at
[Create an AI-optimized Slurm cluster][aihc-slurm].

[aihc-slurm]: https://cloud.google.com/ai-hypercomputer/docs/create/create-slurm-cluster

## Selective Deployment and Destruction using --only and --skip flags

You can control which groups in a blueprint are deployed or destroyed using the `--only` and `--skip` flags with the `gcluster deploy` and `gcluster destroy` commands. This is useful for saving time by not acting on components unnecessarily or for more granular control over resources.

A blueprint is divided into logical groups (the exact group names depend on the blueprint YAML). Check the blueprint file (e.g., `...-blueprint.yaml`) for the exact group names for that blueprint.

### `--only <group1>,<group2>,...`

Use the `--only` flag to have the command act on only the specified, comma-separated groups. Other groups will be untouched.

**Examples:**

- Deploy only the `base` group:

```bash
./gcluster deploy -d a3high-slurm-deployment.yaml examples/machine-learning/a3-highgpu-8g/a3high-slurm-blueprint.yaml --only base
```

- Destroy only the `image` group:

```bash
./gcluster destroy deployment-name --only image
```

- Deploy only the `base` and `cluster` groups:

```bash
./gcluster deploy -d a3high-slurm-deployment.yaml examples/machine-learning/a3-highgpu-8g/a3high-slurm-blueprint.yaml --only base,cluster
```

### `--skip <group1>,<group2>,...`

Use the `--skip` flag to have the command act on all groups except those specified in the comma-separated list.

**Examples:**

- Deploy everything except the `image` group:

```bash
./gcluster deploy -d a4high-slurm-deployment.yaml examples/machine-learning/a4-highgpu-8g/a4high-slurm-blueprint.yaml --skip image
```

- Destroy everything except the `base` group:

```bash
./gcluster destroy deployment-name --skip base
```

**Use cases:**

- Faster iteration: When developing, only deploy the group you are modifying (e.g., `--only base`).
- Partial teardown: Selectively destroy parts of a deployment without affecting others (e.g., `--only image` to remove image but keep networking and other things).
- Avoiding unchanged parts: Use `--skip` to not redeploy parts you know are stable or should be preserved (e.g., `--skip cluster,image`).
- Retry failed operations: If a `deploy` or `destroy` fails on a specific group, rerun the command targeting just that group using `--only`.

**Notes:**
- The exact group names vary by blueprint. Always consult the blueprint YAML used in the command for the correct names.
- The initial deployment may have additional constraints (for example, images may always need to be built the first time). Check the blueprint-specific README for blueprint-specific restrictions.
