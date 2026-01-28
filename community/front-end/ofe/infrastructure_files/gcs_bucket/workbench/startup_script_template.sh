#!/bin/bash
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

if [ ! -d /tmp/jupyterhome/home ]; then ln -s /home /tmp/jupyterhome/; fi

echo "modifying jupyter config" | tee -a /tmp/startup.log

cat >>/tmp/jupyterhome/.jupyter/jupyter_notebook_config.py <<+
jupyter_user = "$USER"
jupyter_home = "/tmp/jupyterhome"
sys.path.append(f"{jupyter_home}/.jupyter/")
c.ServerApp.notebook_dir = "/tmp/jupyterhome"
+

echo "modifying jupyter service" | tee -a /tmp/startup.log
cat >/lib/systemd/system/jupyter.service <<+
[Unit]
Description=Jupyter Notebook Service

[Service]
Type=simple
PIDFile=/run/jupyter.pid
MemoryHigh=3493718272
MemoryMax=3543718272
ExecStart=/bin/bash --login -c '/opt/conda/bin/jupyter lab --config=/tmp/jupyterhome/.jupyter/jupyter_notebook_config.py'
#User=jupyter
User=$USER
Group=$USER
WorkingDirectory=/tmp/jupyterhome
Restart=always

[Install]
WantedBy=multi-user.target
+

echo "reloading and restarting service" | tee -a /tmp/startup.log
systemctl daemon-reload
service jupyter restart

echo BRINGUP COMPLETE
