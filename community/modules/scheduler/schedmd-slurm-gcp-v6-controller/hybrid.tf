# Copyright 2025 "Google LLC"
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

locals {
  output_dir = var.enable_hybrid ? try(abspath(coalesce(var.hybrid_conf.output_dir, ".")), abspath(".")) : null
}

resource "local_file" "hybrid_install" {
  count           = var.enable_hybrid ? 1 : 0
  content         = <<EOF
#!/bin/bash
set -e

OUT_DIR=${local.output_dir}
SCRIPTS_DIR=${module.slurm_files.scripts_dir}
export SLURM_CONFIG_YAML=$OUT_DIR/config.yaml
cd $OUT_DIR
echo "Installing dependencies"
pip install -r $SCRIPTS_DIR/requirements.txt > pip_install.log 2>&1
echo "Generating config files"
python3 $SCRIPTS_DIR/setup.py --hybrid --bucket ${module.slurm_files.slurm_bucket_path}
echo "Extracting scripts"
mkdir -p scripts
unzip -o slurm-gcp-devel.zip -d scripts > /dev/null
#fix the timestamps
find scripts -exec touch {} +
mv config.yaml .config.hash scripts/
chmod -R 700 scripts
echo "Generating config.tgz"
echo "Merge the conf files with your own conf files (integrate cloud_gres.conf into gres.conf and cloud_topology.conf into topology.conf) and move the files in the scripts directory to what you specified in the hybrid_conf.install_dir" > README
tar czf config.tgz --exclude="*.log" --exclude="install_hybrid.sh" --exclude="*.zip" . > /dev/null 2>&1
echo "Success"
EOF
  filename        = "${local.output_dir}/install_hybrid.sh"
  file_permission = "0750"
}
