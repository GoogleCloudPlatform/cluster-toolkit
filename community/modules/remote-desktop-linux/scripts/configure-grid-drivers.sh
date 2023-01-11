#!/bin/bash

# Ensure nvidia grid drivers and other binaries are installed
PACKAGES="build-essential gdebi-core mesa-utils lightdm"

DEBIAN_FRONTEND=noninteractive sudo apt update && sudo apt-get install --assume-yes $PACKAGES

# Download and install Nvidia Grid GPU driver
DRIVER_URL="https://storage.googleapis.com/nvidia-drivers-us-public/GRID/vGPU14.2/NVIDIA-Linux-x86_64-510.85.02-grid.run"
FILE_PATH="/tmp/grid_driver.run"

wget "$DRIVER_URL" -O "$FILE_PATH"
sudo chmod 755 "$FILE_PATH"

sudo gdebi "$FILE_PATH" --non-interactive

# Configure VirtualGL for X
sudo vglserver_config +glx +s +f -t

sudo systemctl set-default graphical.target

# Configure lightdm for X
sudo bash -c 'echo "/usr/sbin/lightdm" > /etc/X11/default-display-manager' 

sudo DEBIAN_FRONTEND=noninteractive DEBCONF_NONINTERACTIVE_SEEN=true dpkg-reconfigure lightdm

sudo systemctl unmask lightdm.service
sudo systemctl daemon-reload
sudo systemctl start lightdm
