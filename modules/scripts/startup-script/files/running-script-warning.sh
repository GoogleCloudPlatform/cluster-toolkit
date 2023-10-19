#!/bin/sh
# Copyright 2023 Google LLC
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

WARNING=$(
	cat <<EOF
** NOTICE **: System services may not be running until startup scripts complete.
The output of the command below will end with "Finished Google Compute Engine 
Startup Scripts." when they are complete. Please review the output for any 
errors which may indicate that services are unhealthy. 

sudo journalctl -b 0 -u google-startup-scripts.service
EOF
)

echo
echo "${WARNING}"
echo
