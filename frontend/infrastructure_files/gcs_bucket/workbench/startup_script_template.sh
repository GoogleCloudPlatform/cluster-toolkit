echo "starting starup script at `date`" | tee -a /tmp/startup.log
mkdir /tmp/jupyterhome 
mkdir /home/$USER
chown $USER:$USER /tmp/jupyterhome

cp /home/jupyter/.jupyter /tmp/jupyterhome/.jupyter -R
chown $USER:$USER /tmp/jupyterhome/.jupyter -R
chown $USER:$USER /home/$USER


#Need to move .jupyter config to a temp dir and create a sudo filesystem under that
echo "modifying jupyter config" | tee -a /tmp/startup.log
echo "jupyter_user = \"$USER\"" >> /tmp/jupyterhome/.jupyter/jupyter_notebook_config.py
echo "jupyter_home = \"/tmp/jupyterhome\"" >> /tmp/jupyterhome/.jupyter/jupyter_notebook_config.py
echo 'sys.path.append(f"{jupyter_home}/.jupyter/")' >> /tmp/jupyterhome/.jupyter/jupyter_notebook_config.py
echo "c.ServerApp.notebook_dir = \"/home/$USER\"" >> /tmp/jupyterhome/.jupyter/jupyter_notebook_config.py

echo "modifying jupyter service" | tee -a /tmp/startup.log
cat > /lib/systemd/system/jupyter.service <<+ 
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

echo "Mounting FileStore filesystem"
sudo apt-get -y update && sudo apt-get install -y nfs-common
