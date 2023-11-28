#Requires -RunAsAdministrator

<#
 # Copyright 2021 Google Inc.
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
#>

# Determine which management interface to use
#
# Get-WmiObject is deprecated and removed in Powershell 6.0+
# https://learn.microsoft.com/en-us/powershell/scripting/whats-new/differences-from-windows-powershell?view=powershell-7#cmdlets-removed-from-powershell
#
# We maintain backwards compatibility with older versions of Powershell by using Get-WmiObject if available
function Get-Mgmt-Command {
    $Command = 'Get-CimInstance'
    if (Get-Command Get-WmiObject 2>&1>$null) {
        $Command = 'Get-WmiObject'
    }
    return $Command
}

# Check if the GPU exists with Windows Management Instrumentation, returning the device ID if it exists
function Find-GPU {
    $MgmtCommand = Get-Mgmt-Command
    try {
        $Command = "(${MgmtCommand} -query ""select DeviceID from Win32_PNPEntity Where (deviceid Like '%PCI\\VEN_10DE%') and (PNPClass = 'Display' or Name = '3D Video Controller')"" | Select-Object DeviceID -ExpandProperty DeviceID).substring(13,8)"
        $dev_id = Invoke-Expression -Command $Command
        return $dev_id
    }
    catch {
        Write-Output "There doesn't seem to be a GPU unit connected to your system."
        return ""
    }
}

# Check if the Driver is already installed
function Check-Driver {
    try {
        &'nvidia-smi.exe'
        Write-Output 'Driver is already installed.'
        Exit
    }
    catch {
        Write-Output 'Driver is not installed, proceeding with installation'
    }
}

# Install the driver
function Install-Driver {

    # Check if the GPU exists and if the driver is already installed
    $gpu_dev_id = Find-GPU

    # Set the correct URL, filename, and arguments to the installer
    $url = 'https://developer.download.nvidia.com/compute/cuda/12.1.1/local_installers/cuda_12.1.1_531.14_windows.exe';
    $file_dir = 'C:\NVIDIA-Driver\cuda_12.1.1_531.14_windows.exe';
    $install_args = '/s /n';
    $os_name = Invoke-Expression -Command 'systeminfo | findstr /B /C:"OS Name"'
    if ($os_name.Contains("Microsoft Windows Server 2016 Datacenter")) {
        # Windows Server 2016 needs an older version of the installer to work properly
        $url = "https://developer.download.nvidia.com/compute/cuda/11.8.0/local_installers/cuda_11.8.0_522.06_windows.exe"
        $file_dir = "C:\NVIDIA-Driver\cuda_11.8.0_522.06_windows.exe"
        # Windows 2016 also requires manual setting of TLS version
        [Net.ServicePointManager]::SecurityProtocol = 'Tls12'
    }
    if ("DEV_102D".Equals($gpu_dev_id)) {
      # K80 GPUs must use an older driver/CUDA version
      $url = 'https://developer.download.nvidia.com/compute/cuda/11.4.0/network_installers/cuda_11.4.0_win10_network.exe';
      $file_dir = 'C:\NVIDIA-Driver\cuda_11.4.0_win10_network.exe';
    }
    if ("DEV_27B8".Equals($gpu_dev_id)) {
      # The latest CUDA bundle (12.1.1) does not support L4 GPUs, so this script
      # only installs the driver (version 528.89). There is a different installer
      # for Windows server 2016/2019/2022 and Windows 10/11, so use systeminfo
      # to determine which installer to use.
      $install_args = '/s /noeula /noreboot';
      if ($os_name.Contains("Server")) {
        $url = 'https://us.download.nvidia.com/tesla/528.89/528.89-data-center-tesla-desktop-winserver-2016-2019-2022-dch-international.exe';
        $file_dir = 'C:\NVIDIA-Driver\528.89-data-center-tesla-desktop-winserver-2016-2019-2022-dch-international.exe';
      } else {
        $url = 'https://us.download.nvidia.com/tesla/528.89/528.89-data-center-tesla-desktop-win10-win11-64bit-dch-international.exe';
        $file_dir = 'C:\NVIDIA-Driver\528.89-data-center-tesla-desktop-win10-win11-64bit-dch-international.exe';
      }
    }
    Check-Driver

    # Create the folder for the driver download
    if (!(Test-Path -Path 'C:\NVIDIA-Driver')) {
        New-Item -Path 'C:\' -Name 'NVIDIA-Driver' -ItemType 'directory' | Out-Null
    }

    # Download the file to a specified directory
    Invoke-WebRequest $url -OutFile $file_dir

    # Install the file with the specified path from earlier as well as the RunAs admin option
    Start-Process -FilePath $file_dir -ArgumentList $install_args -Wait
}

# Run the functions
Install-Driver
