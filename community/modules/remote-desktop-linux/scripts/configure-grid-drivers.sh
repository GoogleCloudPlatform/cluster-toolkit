#!/bin/bash

# Ensure nvidia grid drivers and other binaries are installed
PACKAGES="build-essential gdebi-core mesa-utils gdm3"

sudo apt update && sudo apt-get install --assume-yes $PACKAGES

# Download and install Nvidia Grid GPU driver
DRIVER_URL="https://storage.googleapis.com/nvidia-drivers-us-public/GRID/vGPU14.2/NVIDIA-Linux-x86_64-510.85.02-grid.run"
DRIVER_PATH="/tmp/grid_driver.run"

wget "$DRIVER_URL" -O "$DRIVER_PATH"
sudo chmod 755 "$DRIVER_PATH"
sudo bash $DRIVER_PATH -silent

# Download and install VirtualGL driver
VIRTUALGL_URL="https://sourceforge.net/projects/virtualgl/files/3.0.2/virtualgl_3.0.2_amd64.deb/download"
VIRTUALGL_PATH="/tmp/virtualgl_3.0.2_amd64.deb"

wget "$VIRTUALGL_URL" -O "$VIRTUALGL_PATH"
sudo chmod 755 "$VIRTUALGL_PATH"
sudo gdebi $VIRTUALGL_PATH --non-interactive

# Configure VirtualGL for X
sudo vglserver_config +glx +s +f -t
sudo systemctl set-default graphical.target
sudo systemctl daemon-reload
sudo systemctl start gdm3

sudo bash -c 'echo "/usr/sbin/gdm3" > /etc/X11/default-display-manager'
