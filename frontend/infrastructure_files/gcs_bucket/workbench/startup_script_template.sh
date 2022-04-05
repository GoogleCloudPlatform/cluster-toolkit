echo "starting starup script at `date`" | tee -a /tmp/startup.log
mkdir /home/$USER
chown $USER:$USER /home/$USER

cp /home/jupyter/.jupyter /home/$USER/.jupyter -R
chown $USER:$USER /home/$USER/.jupyter -R

echo "modifying jupyter config" | tee -a /tmp/startup.log
echo "jupyter_user = \"$USER\"" >> /home/$USER/.jupyter/jupyter_notebook_config.py
echo "jupyter_home = \"/home/$USER\"" >> /home/$USER/.jupyter/jupyter_notebook_config.py
echo 'sys.path.append(f"{jupyter_home}/.jupyter/")' >> /home/$USER/.jupyter/jupyter_notebook_config.py
echo "c.ServerApp.notebook_dir = \"/home/$USER\"" >> /home/$USER/.jupyter/jupyter_notebook_config.py

echo "modifying jupyter service" | tee -a /tmp/startup.log
cat > /lib/systemd/system/jupyter.service <<+ 
[Unit]
Description=Jupyter Notebook Service

[Service]
Type=simple
PIDFile=/run/jupyter.pid
MemoryHigh=3493718272
MemoryMax=3543718272
ExecStart=/bin/bash --login -c '/opt/conda/bin/jupyter lab --config=/home/$USER/.jupyter/jupyter_notebook_config.py'
#User=jupyter
User=$USER
Group=$USER
WorkingDirectory=/home/$USER
Restart=always

[Install]
WantedBy=multi-user.target
+

echo "reloading and restarting service" | tee -a /tmp/startup.log
systemctl daemon-reload
service jupyter restart

echo "Mounting FileStore filesystem"
sudo apt-get -y update && sudo apt-get install -y nfs-common

mkdir /home/$USER/mount_points
