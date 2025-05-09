# Copyright 2025 Google LLC
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

import requests
import os
from google.cloud import secretmanager_v1
from google.api_core.exceptions import NotFound
from typing import Optional, Dict
import jinja2
import datetime
import pathlib

def is_valid_path(input_path: str, expected_prefix: str) -> bool:
    input_path = os.path.abspath(input_path)
    return os.path.isabs(input_path) and input_path.startswith(expected_prefix)


class AF3SlurmClient:
    """An AF3 client for interacting with the Slurm API."""

    def __init__(self, remote_host: str, remote_port: int, gcp_project_id: str, gcp_secret_name: str, af3_config: Dict)->None:
        self.base_url = f"http://{remote_host}:{remote_port}/slurm/v0.0.41"
        self.gcp_project_id = gcp_project_id
        self.gcp_secret_name = gcp_secret_name
        self.af3_config = af3_config

    def __get_headers(self) -> Optional[Dict]:
        client = secretmanager_v1.SecretManagerServiceClient()
        secret_path = f"projects/{self.gcp_project_id}/secrets/{self.gcp_secret_name}/versions/latest"

        try:
            response = client.access_secret_version(name=secret_path)
            token = response.payload.data.decode("UTF-8")
            print("[INFO] Retrieved token from Secret Manager.")
            return {"X-SLURM-USER-TOKEN": token}
        except NotFound:
            print(f"[ERROR] Secret '{self.gcp_secret_name}' not found.")
            raise NotFound(
                f"[ERROR] Secret '{self.gcp_secret_name}' not found.")
        except Exception as e:
            print(f"[ERROR] Failed to retrieve token: {e}")
            raise Exception(f"[ERROR] Failed to retrieve token: {e}")

    def __retrieve_url(self, endpoint: str)-> str:
        return f"{self.base_url}/{endpoint}"

    def ping(self):
        """Pings the Slurm API server."""
        url = self.__retrieve_url("ping")
        try:
            headers = self.__get_headers()
            response = requests.get(url, headers=headers)
            response.raise_for_status()
            print(f"Ping response code : {response.status_code}")
            return response.json()
        except requests.exceptions.RequestException as e:
            print(f"Error pinging Slurm API: {e}")
            return None

    def _render_template(self, config_options: dict) -> str:
        """Renders the Jinja2 template with af3_config values."""
        env = jinja2.Environment(loader=jinja2.FileSystemLoader(
            os.path.dirname(self.af3_config["job_template_path"])))
        template = env.get_template(
            os.path.basename(self.af3_config["job_template_path"]))
        rendered = template.render(config_options)
        return rendered

    def submit_job(self, job_config: dict, job_command: str)->Optional[Dict]:
        """Submits a job to Slurm server."""
        url = self.__retrieve_url("job/submit")
        submit_input = {"job": job_config, "script": job_command}
        try:
            headers = self.__get_headers()
            response = requests.post(url, headers=headers, json=submit_input)
            response.raise_for_status()
            return response.json()
        except requests.exceptions.RequestException as e:
            print(f"Error submitting job to Slurm: {e}")
            return None

    def submit_base_job(self, job_config: dict,job_type:str, input_file: str, output_path: Optional[str] = None)->Optional[Dict]:
        """Submits a data pipeline job to Slurm server."""
        timestamp = datetime.datetime.now().strftime("%Y%m%d_%H%M%S")
        base_dir = f"{job_type}_output"
        if output_path is None:
            # Generate a timestamped output path if not provided
            output_path = os.path.join(self.af3_config["default_folder"], base_dir, f"{timestamp}", os.path.splitext(
                os.path.basename(input_file))[0])

        if not is_valid_path(output_path, self.af3_config["default_folder"]):
            raise ValueError(
                f"Output file path '{output_path}' is not valid. It should be absolute and start with '{self.af3_config['input_prefix']}'.")
        
        file_name = pathlib.Path(input_file).stem
        script_options = {**self.af3_config, ** {
            "input_path": os.path.join(self.af3_config["default_folder"],input_file),
            "output_path": output_path,
            "job_type": job_type,
            "af3_log_base_dir": os.path.join(self.af3_config["default_folder"],base_dir,f"{timestamp}",file_name,"slurm_logs"),
        }}
        updated_job_config = {**job_config, ** {
            "standard_output": os.path.join(script_options["af3_log_base_dir"], "job_%j","out.txt"),
            "standard_error": os.path.join(script_options["af3_log_base_dir"], "job_%j","err.txt"),
        }}
        script = self._render_template(script_options)
        submit_result =  self.submit_job(updated_job_config, script)
        print(f"[INFO] Submitted Job output path: {output_path}")
        return submit_result


    def submit_inference_job(self, job_config: dict, input_file: str, output_path: Optional[str] = None)->Optional[Dict]:
        """Submits an inference job to Slurm server."""
        return self.submit_base_job(job_config, "inference", input_file, output_path)
    
    def submit_data_pipeline_job(self, job_config: dict, input_file: str, output_path: Optional[str] = None)->Optional[Dict]:
        """Submits an data pipeline job to Slurm server."""
        return self.submit_base_job(job_config, "datapipeline", input_file, output_path)

    def cancel_job(self, job_id)->Optional[Dict]:
        """Cancels a job on the Slurm server."""
        url = self.__retrieve_url(f"job/{job_id}/cancel")
        try:
            headers = self.__get_headers()
            response = requests.post(url, headers=headers)
            response.raise_for_status()
            return response.json()
        except requests.exceptions.RequestException as e:
            print(f"Error canceling job {job_id} on Slurm: {e}")
            return None

    def get_job_info(self, job_id)->Optional[Dict]:
        """Retrieves information about a specific job."""
        url = self.__retrieve_url(f"job/{job_id}")
        try:
            headers = self.__get_headers()
            response = requests.get(url, headers=headers)
            response.raise_for_status()
            return response.json()
        except requests.exceptions.RequestException as e:
            print(f"Error retrieving job {job_id} from Slurm: {e}")
            return None

    def get_all_jobs(self)->Optional[Dict]:
        """Retrieves information about all jobs."""
        url = os.path.join(self.base_url, "jobs")
        url = self.__retrieve_url("jobs")
        try:
            headers = self.__get_headers()
            response = requests.get(url, headers=headers)
            response.raise_for_status()
            return response.json()
        except requests.exceptions.RequestException as e:
            print(f"Error retrieving all jobs from Slurm: {e}")
            return None
