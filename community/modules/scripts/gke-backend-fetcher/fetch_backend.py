#!/usr/bin/env python3
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import sys
import json
import subprocess
import time
import shutil

def check_dependencies():
    missing = []
    if not shutil.which("gcloud"):
        missing.append("gcloud")
    if not shutil.which("kubectl"):
        missing.append("kubectl")
    
    if missing:
        sys.stderr.write(f"{{\"error\": \"Missing required host dependencies: {', '.join(missing)}. Please install them and ensure they are in your PATH.\"}}\n")
        sys.exit(1)

def main():
    check_dependencies()

    try:
        input_data = json.loads(sys.stdin.read())
    except Exception as e:
        sys.stderr.write(f"{{\"error\": \"Failed to read stdin: {e}\"}}\n")
        sys.exit(1)

    project_id = input_data.get('project_id')
    cluster_name = input_data.get('cluster_name')
    location = input_data.get('location')
    namespace = input_data.get('namespace')
    service_name = input_data.get('service_name')
    service_port = str(input_data.get('service_port'))
    timeout_seconds = int(input_data.get('timeout_seconds', '600'))

    # Configure kubectl
    subprocess.run([
        'gcloud', 'container', 'clusters', 'get-credentials', cluster_name,
        '--project', project_id, '--region', location
    ], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    pattern = f"-{namespace}-{service_name}-{service_port}-"
    initial_start_time = time.time()
    backend_filter = ""
    
    while (time.time() - initial_start_time) < timeout_seconds:
        try:
            res = subprocess.run(['kubectl', 'get', 'ingress', '-n', namespace, '-o', 'json'], capture_output=True, text=True)
            if res.returncode == 0:
                ingresses = json.loads(res.stdout).get('items', [])
                for ingress in ingresses:
                    annotations = ingress.get('metadata', {}).get('annotations', {})
                    backends_str = annotations.get('ingress.kubernetes.io/backends', '{}')
                    
                    try:
                        backends = json.loads(backends_str)
                    except json.JSONDecodeError:
                        continue
                    
                    matching_backends = [k for k in backends.keys() if pattern in k or f"k8s-be-{service_port}--" in k]
                    
                    if len(matching_backends) == 1:
                        backend_filter = f"name~^{matching_backends[0]}"
                        break
                    elif len(matching_backends) > 1:
                        sys.stderr.write(f"{{\"error\": \"Multiple backend services found for {service_name}:{service_port}\"}}\n")
                        sys.exit(1)
            
            if backend_filter:
                break
        except Exception:
            pass
            
        time.sleep(10)

    if not backend_filter:
        sys.stderr.write(f"{{\"error\": \"Timeout waiting for backend service annotation for {service_name}:{service_port}\"}}\n")
        sys.exit(1)

    backend_service_id = ""
    real_backend_name = ""

    while (time.time() - initial_start_time) < timeout_seconds:
        try:
            res = subprocess.run([
                'gcloud', 'compute', 'backend-services', 'list',
                '--project', project_id, f'--filter={backend_filter}', '--format=json'
            ], capture_output=True, text=True)
            
            if res.returncode == 0:
                services = json.loads(res.stdout)
                if len(services) == 1:
                    backend_service_id = services[0].get('id')
                    real_backend_name = services[0].get('name')
                    break
                elif len(services) > 1:
                    sys.stderr.write(f"{{\"error\": \"Multiple GCP backend services matched filter {backend_filter}\"}}\n")
                    sys.exit(1)
        except Exception:
            pass
            
        time.sleep(10)

    if not backend_service_id:
        sys.stderr.write("{\"error\": \"Timeout fetching GCP backend service ID\"}\n")
        sys.exit(1)

    sys.stdout.write(json.dumps({
        "backend_service_id": str(backend_service_id),
        "backend_service_name": real_backend_name
    }) + "\n")

if __name__ == "__main__":
    main()
