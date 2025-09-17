# Setup: Setup AF3 with Simple Ipynb Launcher - PART 2: IPython notebook setup
This guide explains how to deploy and access the Jupyter Notebook environment for AlphaFold **after the af3-slurm.yaml cluster has been deployed** with **SLURM REST support enabled**. If you have not followed the steps outlined in [Setup-pre-cluster-deployment.md](./Setup-pre-cluster-deployment.md) before, please do so before continuing.

## Prerequisites
Before proceeding, ensure the following configuration values were set correctly in your `af3-slurm-deployment.yaml` file **prior to deploying the SLURM cluster**:

```yaml
slurm_rest_server_activate: true
slurm_rest_token_secret_name: "<your-secret-name>"            # Name of your Secret Manager secret
af3ipynb_bucket: "<your-pre-existing-bucket-name>"           # Existing Cloud Storage bucket name
```

## Deploying the Jupyter Notebook Blueprint

### 1. Upload the required Notebook to the Cloud Storage Bucket
Access the controller node of your deployed cluster (replace placeholders `<controller-node-name>` and `<your-zone>` with your actual node and zone):

```bash
gcloud compute ssh <controller-node-name> --zone=<your-zone>
```

On the controller node, run the following command (assuming the `slurm_rest_user` value in `af3-slurm-deployment.yaml` has not changed):

```bash
cd /home/af3ipynb/ipynb_setup
ansible-playbook ipynb-upload-config.yml
```

This step uploads the notebook (`slurm-rest-api-notebook.ipynb`) along with its required scripts and libraries to the bucket defined in the `af3ipynb_bucket` variable in `af3-slurm-deployment.yaml` file.

### 2. Grant Secret Access to the Notebook's Service Account
**Where to run**: **On the system where you executed `gcluster`** (where the `gcloud` CLI is authenticated with access to your GCP project).

This setting ensures that the notebook server can successfully retrieve the specified secret by name. For this you need to make sure that the service account running the Jupyter Notebook instance (typically the Compute Engine default service account) has permission to access the Secret Manager secret that stores your SLURM REST token.

The following command grants the Compute Engine default service account the `roles/secretmanager.secretAccessor` role, allowing it to access the specified secret in Secret Manager. The attached condition always evaluates to true, ensuring access is consistently granted.

```bash
gcloud secrets add-iam-policy-binding <your-secret-name> \
--member="serviceAccount:$(gcloud projects describe <your-project-id> --format='value(projectNumber)')-compute@developer.gserviceaccount.com" \
--role="roles/secretmanager.secretAccessor" \
--condition="expression=true,title=AlwaysTrue,description=Allow access to Secret Manager"
```

You can verify this configuration by navigating to Secret Manager in the Google Cloud Console
. To access it:

 1. Open the Google Cloud Console

 2. Select your project from the top project selector if it's not already selected.

 3. In the left-hand navigation menu, go to <b>Security</b> > <b>Secret Manager</b>.

 4. You will see a list of secrets—verify that your secret (e.g., API key, database credentials) is listed and properly configured.

Tip: If you don't see <b>"Secret Manager"</b> in the navigation, use the search bar at the top to search for  <b>"Secret Manager"</b> directly.


### 3. Deploy the Notebook Environment
**Where to run**: **On the system where you executed `gcluster`**, under your `cluster-toolkit` directory.

Deploy the Jupyter Notebook environment using the following command:

```bash
# Move to cluster-toolkit root folder
cd cluster-toolkit
./gcluster deploy -d examples/science/af3-slurm/af3-slurm-deployment.yaml \
examples/science/af3-slurm/examples/simple_ipynb_launcher/af3-slurm-ipynb.yaml --auto-approve
```

### 4. Access the Notebook via Vertex AI Workbench
In the Google Cloud Console:

1. Navigate to `Vertex AI` → `Workbench` → `Instances`

2. Open the JupyterLab interface for the newly deployed instance

3. Locate and open the `slurm-rest-api-notebook.ipynb` file. If you haven't modified the default value of `af3ipynb_bucket_local_mount` in the `af3-slurm-deployment.yaml`, the notebook files will be mounted to `/home/jupyter/alphafold` on the Jupyter notebook system.

    When you first connect to Jupyter Notebook, it only shows the alphafold folder — this can appear as if it's located at `/alphafold`. In reality, Jupyter starts in the `/home/jupyter` directory, and the `alphafold` folder is located inside it.

    So while it looks like `/alphafold` in the interface, the actual path is `/home/jupyter/alphafold`.

### 5. Verify REST Token Access
**Where to run**: **On the system where you executed `gcluster`** (where the `gcloud` CLI is authenticated with access to your GCP project, similar to step [Grant Secret Access to the Notebook's Service Account](#2-grant-secret-access-to-the-notebooks-service-account)
).

To verify that Secret Manager access is properly configured:

1. Open a terminal in JupyterLab:
   - Go to **File > New Launcher**, or
   - Click the **“+” button** under the **File** bar,

   then click **“Terminal”** under the **Other** section.

2. Then, run the following command in the terminal:

   ```bash
   gcloud secrets versions access latest --secret=<your-secret-name>
    ```

    Replace `<your-secret-name>` with the name of your secret.

If the command returns the secret value successfully, it confirms that the notebook environment can securely access the SLURM REST API. When you run the relevant section under the <b>AF3 - SLURM REST API</b> header in the notebook, you should see a log message similar to:

```log
[INFO] Token retrieved from Secret Manager successfully.
```

This indicates that the token was successfully retrieved from Secret Manager and that the authentication setup is functioning correctly.

## Using the environment
Go to [Ipynb.md](./Ipynb.md) for documentation on how to use the IPython environment.

## Customization
You can adjust the notebook setup behavior using blueprint variables in the deployment YAML.
All configurations should be validated before running jobs.
If further modifications to SLURM REST/API Server behavior are required, you must destroy and redeploy the cluster with the updated settings.

## Teardown
To remove the Jupyter Notebook deployment when it is no longer needed, run the following command:

    ```bash
    ./gcluster destroy af3-slurm-ipynb --auto-approve
    ```

    > [!WARNING]
    > If you do not destroy the Jupyter Notebook deployment, it may continue to incur costs.
    > Additionally, any Cloud Storage buckets you created (via the CLI or console) will not be automatically deleted. You are responsible for cleaning them up manually to avoid unnecessary charges.
    > For deleting the buckets consult [Delete buckets](https://cloud.google.com/storage/docs/deleting-buckets).
