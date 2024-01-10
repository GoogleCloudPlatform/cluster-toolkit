# Monte Carlo Simulations for Value at Risk
This tutorial demonstrates how to run Monte Carlo simulations on Google Cloud's
Batch and how to analyze the data using Vertex AI Notebooks.

Monte Carlo simulation is a mathematical technique that can be used to estimate
the
[value at risk (VaR) of a financial portfolio](https://en.wikipedia.org/wiki/Value_at_risk).
The basic idea is to generate a large number of random price paths for the
assets in the portfolio, and then calculate the VaR as the minimum loss that
would be expected to occur with a certain probability.

1. Choose a time horizon and a confidence level.
1. Generate a large number of random price paths (random walks) for the assets
   in the portfolio.
1. Calculate the portfolio value at the end of each price path.
1. Determine the VaR as the minimum portfolio value that occurs with the
   specified confidence level.

## Cost to run
The following elements of this tutorial will result in charges to your billing
account. Please validate with the
[Google Cloud Pricing Calculator](https://cloud.google.com/products/calculator)

* Batch
* Pub/Sub
* BigQuery
* Vertex AI Notebooks

## Tutorial architecture
The overall structure of this tutorial is as follows:

* The Monte Carlo simulation is managed with
  [Batch](https://cloud.google.com/batch/docs/get-started).
* The output of the Monte Carlo simulation is published to a
  [PubSub](https://cloud.google.com/pubsub/docs/overview) topic.
* The PubSub data is entered into [BigQuery](https://cloud.google.com/bigquery)
via a [PubSub BigQuery subscription](https://cloud.google.com/pubsub/docs/bigquery)
* The data is visualized via a
  [Vertex AI Workbench](https://cloud.google.com/vertex-ai-workbench)
  [Jupyter Notebook](https://jupyter.org/)

<img src="https://services.google.com/fh/files/blogs/fsi_architecture.png" width="800" />

## Basic getting started

1. Get the HPC Toolkit configured.

Build the `ghpc` binary:

```shell
git clone https://github.com/GoogleCloudPlatform/hpc-toolkit
cd hpc-toolkit
make
./ghpc --version
./ghpc --help
```

2\. Run `ghpc` on the blueprint `fsi-montecarlo-on-batch.yaml`

```bash
./ghpc create community/examples/fsi-montecarlo-on-batch.yaml \
   --vars "project_id=${GOOGLE_CLOUD_PROJECT}"
```

Where `GOOGLE_CLOUD_PROJECT` has been set via an export command

```shell
export GOOGLE_CLOUD_PROJECT=my_project_id
```

If successful, you will see output similar to:

<blockquote>
<p>
To deploy your infrastructure please run:

./ghpc deploy fsimontecarlo
</p>
</blockquote>

3\. Deploy the blueprint as instructed:

```bash
./ghpc deploy fsimontecarlo
```

If successful, this will prompt you:

```bash
Summary of proposed changes: Plan: 22 to add, 0 to change, 0 to destroy.
(D)isplay full proposed changes,
(A)pply proposed changes,
(S)top and exit,
(C)ontinue without applying
Please select an option [d,a,s,c]:
```

Respond with `apply`, "a". You may be asked to respond twice.

When the job is complete it will indicate `Succeeded`. You can then proceed to
the next section.

At this point, all the required infrastructure has been deployed.

## Open Vertex AI Workbench

1\. Go to the Vertex AI Workbench Notebooks instances in the Google Cloud Console:

https://console.cloud.google.com/vertex-ai/workbench/user-managed

2\. Open JupyterLab on the Notebook instance listed.

<img src="https://services.google.com/fh/files/blogs/fsi_workbench.png" width="500" />

```bash
Click on `OPEN JUPYTERLAB` link
```

3\. In the JupyterLab UI, you will see a list of directories:

```bash
Select `fsi`
```

Under `fsi` all the files required to run the demo have been pepared.

<img src="https://services.google.com/fh/files/blogs/fsi_files.png" width="300" />

4\. Open a terminal window by clicking on the terminal icon.

<img src="https://services.google.com/fh/files/blogs/fsi_terminal.png" width="200" />

5\. Update the local Python requirements:

```bash
python3 -m pip install --user -r requirements.txt
```

There may be some incompatibilities listed, but it will not affect this demo.

6\. Run the `batch.py` Python script to ensure it is working.

```bash
python3 batch.py --help
``` 

You will see a listing of the help messages

7\. To start the VaR simulation, run `batch.py` with the config file and
   `--create_job`

```bash
python3 batch.py --config_file mc_run.yaml --create_job
```

You should see output without any errors, listing information about the job.

> To see if the job is running, use the `--list_jobs` options.

```bash
python3 batch.py --config_file mc_run.yaml --list_jobs
```

If you want to see the jobs listed in the Cloud Console, you can click:

https://console.cloud.google.com/batch/jobs

## View the data in BigQuery
The batch job runs Monte Carlo simulation for the VaR calculation. The output
from each run is stored in BigQuery. To view this data in it's raw form, you can
view BigQuery in the Cloud Console:

https://console.cloud.google.com/bigquery

> Navigate to the `fsi_table`.

<img src="https://services.google.com/fh/files/blogs/fsi_bq.png" width="300" />

> There you can see the schema.

<img src="https://services.google.com/fh/files/blogs/fsi_schema.png" width="600" />

> To see the data, click on `PREVIEW`

<img src="https://services.google.com/fh/files/blogs/fsi_preview.png" width="600" />

For an advanced user, you can run queries directly in the BigQuery UI.

## Visualization in the Notebook

Finally, you can select the `FSI_MonteCarlo.ipynb` from the left navigation in
the JupyterLab window.

<img src="https://services.google.com/fh/files/blogs/fsi_ipynb.png" width="300" />

To run the cells in the notebook, select the cell, then click the play button,
or `Alt-Enter`.

> Run the cells in the order they appear.

<img src="https://services.google.com/fh/files/blogs/fsi_notebook.png" width="600" />

> After running the second cell, you should see output.

<img src="https://services.google.com/fh/files/blogs/fsi_output.png" width="500" />

> Finally, when you run Cell #4, you will see graphs and table summaries.

<img src="https://services.google.com/fh/files/blogs/fsi_graphs.png" width="600" />

## Summary

In this tutorial, you accomplished the following:

* You created Cloud infrastructure
  * Vertex AI notebooks
  * BigQuery Tables
  * Pubsub BigQuery Subscription
  * Batch jobs
* You ran a MonteCarlo simulation for VaR on several stock tickers.
* You reviewed the data in BigQuery
* You visualized the data in Vertex AI notebooks.

## Shutting down

The best way to clean up your workspace is to delete the project. This will
ensure you are not billed for any of the Cloud usage.

### Alternatively

The other choice is to run a `ghpc destroy` command.

```bash
./ghpc destroy fsimontecarlo
```
