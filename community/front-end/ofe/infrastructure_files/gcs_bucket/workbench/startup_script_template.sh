#!/bin/bash

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
