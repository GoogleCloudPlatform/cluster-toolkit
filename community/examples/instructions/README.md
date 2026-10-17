# Custom Deployment Instructions Example

The [instructions-demo.yaml](instructions-demo.yaml) blueprint demonstrates how to use the [`instructions`](../../modules/general/instructions/README.md) community module to provide dynamic, formatted deployment guidance and next steps upon cluster creation.

In this example, the blueprint:

1. Renders a template file ([instructions.txt.tftpl](instructions.txt.tftpl)) with deployment variables (`deployment_name`, `cluster_name`, `admin_user`).
2. Wraps the instructions in a decorative banner titled `"Cluster Setup Complete"`.
3. Emits the formatted instructions to the standard `instructions` output for display in the terminal at the end of deployment.
4. Writes the rendered instructions locally to `NEXT_STEPS.txt` inside the deployment directory for ongoing reference.

## Deploy the Blueprint

Create and deploy the blueprint from the root of the Cluster Toolkit repository:

```bash
./gcluster create community/examples/instructions/instructions-demo.yaml \
  --vars project_id=<PROJECT_ID> -d deployment.yaml
./gcluster deploy instructions-test
```

## Verify Instructions

During and after deployment:

1. **Terminal Output**: Check the deployment completion summary for the banner and formatted next steps.
2. **Local File**: Inspect the generated file inside the deployment directory:

   ```bash
   cat instructions-test/NEXT_STEPS.txt
   ```

## Clean Up

To destroy the deployment resources and remove local state:

```bash
./gcluster destroy instructions-test
```
