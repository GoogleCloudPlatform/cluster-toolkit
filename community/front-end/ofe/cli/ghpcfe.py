# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""The Command Line Interface to access the Cluster Toolkit FrontEnd"""

import click
import requests
import sys
import yaml
import json
from pathlib import Path

import utils
from utils import notimplementedyet


def unified_error_handling(func):
    def func_wrapper(*args, **kwargs):
        try:
            return func(*args, **kwargs)
        except requests.HTTPError as e:
            status_code = e.response.status_code
            print(e)
            print("------")
            if status_code==404:
                print("The server has returned an HTTP 404 error, which "\
                      "normally indicates a non-existing/invisible resource, "\
                      "e.g., requesting resource with an invalid ID.")
            elif status_code==403:
                print("The server has returned an HTTP 403 error, which "\
                      "normally indicates a permission problem, e.g., current "\
                      "user has no permission to access requested resource.")
            else:
                print("The server has returned an HTTP " + str(status_code) +
                      " error.")
            return None
        except:
            print("------")
            print("Unexpected error:", sys.exc_info()[0])
            raise
    return func_wrapper


@click.group("top-level")
def cli():
    pass

@cli.command("config")
def config():
    """Initialise this application.

    Perform a one-time initialisation of this application.
    Configurations are saved in $HOME/.ghpcfe/config file.
    """
    print("Configuration file will be written at $HOME/.ghpcfe/config")
    print()
    server = input("Enter the URL of the Cluster Toolkit FrontEnd website: ")
    try:
        requests.get(server, timeout=10)
    # pylint: disable=unused-variable
    except requests.ConnectionError as exception:
        print("URL appears to be invalid. Please try again.")
        sys.exit(1)
    api_key = input("Enter the API key associated with your user account: ")
    try:
        if len(api_key) != 40:
            raise ValueError("Invalid length.")
        # pylint: disable=unused-variable
        value = int(api_key, 16)
    except ValueError as exception:
        print("The API key should be a 40-digit hexadecimal string.")
        print("It can be found from the associated website.")
        sys.exit(2)
    config_data = {
      "config": {
        "server": {
          "url": server,
          "accessKey": api_key
        }
      }
    }
    config_dir = str(Path.home()) + "/.ghpcfe"
    p = Path(config_dir)
    p.mkdir(parents=True, exist_ok=True)
    filepath = p / "config"
    with filepath.open("w+") as file:
        yaml.dump(config_data, file, default_flow_style=False)


# credential management

@cli.group("credential")
def credential():
    """Manage credentials.

    Manage cloud credentials used with this system.
    """
    pass

@credential.command(name="list", short_help="List all existing credentials.")
@unified_error_handling
def credential_list():
    """List all existing credentials."""
    cfg = utils.load_config()
    ret = utils.get_model_state(cfg, "credentials")
    parsed = json.loads(ret)
    utils.print_json(json.dumps(parsed))

@credential.command(name="add",
                    short_help="Add/register a credential to the system.")
@click.option("-n", "--name", required=True, type=click.STRING)
@click.option("-f", "--credential_file", required=True,
              type=click.File(mode="r"))
def credential_add(name, credential_file):
    """Add/register a credential to the system."""
    cfg = utils.load_config()
    cred = credential_file.read()
    url = f"{config['server']['url']}/api/credential-validate"
    data = {"cloud_provider": "GCP", "detail": cred}
    headers = {"Authorization": f"Token {config['server']['accessKey']}"}
    response = requests.post(url, data=data, headers=headers)
    parsed = json.loads(response.text)
    if parsed["validated"]:
        data["name"] = name
        ret = utils.model_create(cfg, "credentials", data)
        parsed = json.loads(ret)
        utils.print_json(json.dumps(parsed))
    else:
        print("Failed to validate this credential on GCP")
        sys.exit(3)

@credential.command(name="delete",
                    short_help="Delete a credential from the system.")
@notimplementedyet
def credential_delete():
    """Delete a credential from the system."""
    pass


# cluster management

@cli.group("cluster")
def cluster():
    """Manage clusters.

    Manage the life cycles of clusters in this system.
    """
    pass

@cluster.command(name="list", short_help="List all existing clusters.")
@unified_error_handling
def cluster_list():
    """List all existing clusters."""
    cfg = utils.load_config()
    ret = utils.get_model_state(cfg, "clusters")
    # excludeded fields from list view
    excluded = ("cloud_region", "cloud_credential", "cloud_vpc", "cloud_subnet",
                "spackdir", "mount_points")
    parsed = json.loads(ret)
    for obj in parsed:
        for key in excluded:
            del obj[key]
    utils.print_json(json.dumps(parsed))

@cluster.command(name="show", short_help="Show details of an existing cluster.")
@click.option("--cluster_id", required=True, type=click.INT)
@unified_error_handling
def cluster_show(cluster_id):
    """Show details of an existing cluster."""
    cfg = utils.load_config()
    ret = utils.get_model_state(cfg, "clusters", cluster_id)
    parsed = json.loads(ret)
    utils.print_json(json.dumps(parsed))


@cluster.command(name="create", short_help="Create a new cluster.")
@click.option("-n", "--name", required=True, type=click.STRING)
@click.option("-s", "--subnet", required=True, type=click.STRING)
@notimplementedyet
#pylint: disable=unused-argument
def cluster_create(name, subnet):
    """Create a new cluster."""
    print(f"cluster {name} created.")

@cluster.command(name="destroy", short_help="Destroy a cluster.")
@notimplementedyet
def cluster_destroy():
    """Destroy a cluster."""
    pass


# application management

@cli.group("application")
def application():
    """Manage applications.

    Manage application installations on clusters.
    """
    pass

@application.command(name="list", short_help="List all applications.")
def application_list():
    click.echo("List all existing applications.")
    cfg = utils.load_config()
    ret = utils.get_model_state(cfg, "applications")
    # excluded fields from list view
    excluded = ("install_loc", "install_partition", "installed_architecture",
                "load_command", "compiler", "mpi")
    parsed = json.loads(ret)
    for obj in parsed:
        for key in excluded:
            del obj[key]
    utils.print_json(json.dumps(parsed))

@application.command(name="show", short_help="Show details of an application.")
@notimplementedyet
def application_show():
    click.echo("Show details of an application.")

@application.command(name="spack-install",
                     short_help="Install a Spack application.")
@notimplementedyet
def application_spack_install():
    """Install a Spack application."""
    pass


# job management

@cli.group("job")
def job():
    """Manage jobs.

    Manage jobs to run applications on clusters.
    """
    pass

@job.command(name="list", short_help="List all existing jobs.")
def job_list():
    """List all existing jobs."""
    cfg = utils.load_config()
    ret = utils.get_model_state(cfg, "jobs")
    # excluded fields in list view
    excluded = ()
    parsed = json.loads(ret)
    for obj in parsed:
        for key in excluded:
            del obj[key]
    utils.print_json(json.dumps(parsed))

@job.command(name="show", short_help="Show details of an existing job.")
@notimplementedyet
def job_show():
    """Show details of an existing job."""
    pass

@job.command(name="submit",
             short_help="Submit a job to run an application on a cluster.")
@notimplementedyet
def job_submit():
    """Submit a job to run a specified application on a cluster."""
    pass


if __name__ == "__main__":
    cli()
