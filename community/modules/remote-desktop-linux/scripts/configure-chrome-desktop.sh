#!/bin/bash

# Ensure desktop is installed
PACKAGES="xfce4 xfce4-goodies"

sudo apt-get install --assume-yes $PACKAGES

# Fix headless Nvidia issue
sudo nvidia-xconfig --query-gpu-info | sudo tee /tmp/gpu-info.txt >/dev/null

PCI_ID=$(grep </tmp/gpu-info.txt "PCI BusID " | head -n 1 | cut -d':' -f2-99 | xargs)
sudo nvidia-xconfig -a --allow-empty-initial-configuration --enable-all-gpus --virtual=1920x1200 --busid="$PCI_ID"
sudo sed -i '/Section "Device"/a \ \ \ \ Option\t"HardDPMS" "false"' /etc/X11/xorg.conf

# Download and Install Chrome Remote Desktop
CRD_URL="https://dl.google.com/linux/direct/chrome-remote-desktop_current_amd64.deb"
FILE_PATH="/tmp/chrome-remote-desktop_current_amd64.deb"

wget "$CRD_URL" -O "$FILE_PATH"

sudo DEBIAN_FRONTEND=noninteractive apt-get install --assume-yes "$FILE_PATH"

sudo bash -c 'echo "exec /etc/X11/Xsession /usr/bin/xfce4-session" > /etc/chrome-remote-desktop-session'

sudo /etc/init.d/chrome-remote-desktop start
