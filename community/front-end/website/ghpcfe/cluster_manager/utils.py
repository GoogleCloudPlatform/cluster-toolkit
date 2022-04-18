# Copyright 2022 Google LLC
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

from pathlib import Path
import json, os, subprocess
import requests
import yaml
import copy

#TODO = Make some form of global config file
g_baseDir = Path( __file__ ).resolve().parent.parent.parent.parent
g_config = {
    'baseDir': g_baseDir,
    'server': {},
    'loaded': False
}



def load_config(configFile=g_baseDir/'configuration.yaml', accessKey=None):
    global g_config
    def _pathify(var):
        global g_config
        if var in g_config:
            if type(g_config[var]) is not Path:
                g_config[var] = Path(g_config[var])

    if not g_config['loaded']:

        with configFile.open('r') as f:
            g_config.update(yaml.safe_load(f)['config'])

        # Convert certain entries to appropriate types
        _pathify('baseDir')

        if accessKey:
            g_config["server"]["accessKey"] = accessKey
        elif "C398_API_AUTHENTICATION_TOKEN" in os.environ:
            g_config["server"]["accessKey"] = os.environ["C398_API_AUTHENTICATION_TOKEN"]

        g_config['loaded'] = True

    if accessKey and (("accessKey" not in g_config["server"]) or (accessKey != g_config["server"]["accessKey"])):
        cfg = copy.deepcopy(g_config)
        cfg["server"]["accessKey"] = accessKey
        return cfg

    return g_config


def _parse_tfvars(filename):
    res = dict()
    with open(filename, 'r') as fp:
        lines = [x for x in fp]
    
    lnum = 0
    multi_line_terminator = None

    while lnum < len(lines):
        line = lines[lnum]
        lnum += 1
        if multi_line_terminator:
            if line.startswith(multi_line_terminator):
                multi_line_terminator = None
                res[current_key] = current_value
            else:
                current_value += line
        else:
            line = line.strip()
            if line.startswith('#'):
                continue
            line = line.split('=', maxsplit=1)
            if len(line) != 2:
                # Not sure what to do when it's not x=y... skip?
                continue
            (current_key, current_value) = [x.strip() for x in line]
    
            if current_value.startswith('<<'):
                multi_line_terminator=current_value[2:]
                current_value = ""
                continue

            res[current_key] = current_value.strip(' " " ')

    return res


def add_host_to_server_firewall(newHost):
    if not newHost:
        return
    config = load_config()

    if "firewall" not in config["server"]:
        return
    if not config["server"]["firewall"].get("update", False):
        return
    firewallName = config["server"]["firewall"].get("name", None)
    if not firewallName:
        return
    if config["server"].get("host_type", None) == "GCP":
        project = config["server"].get("gcp_project", None)
        if project:
            # Only support GCP at the moment
            try:
                import googleapiclient.discovery
                gcloud = googleapiclient.discovery.build("compute", "v1", cache_discovery=False)
                existingFW = gcloud.firewalls().get(project=project, firewall=firewallName).execute()
                patch = {'sourceRanges': existingFW["sourceRanges"]}
                patch['sourceRanges'].append(f"{newHost}/32")
                res = gcloud.firewalls().patch(project=project, firewall=firewallName, body=patch).execute()
            except Exception as e:
                # TODO: Log
                raise

def remove_host_from_server_firewall(tgtHost):
    config = load_config()

    if "firewall" not in config["server"]:
        return
    if not config["server"]["firewall"].get("update", False):
        return
    firewallName = config["server"]["firewall"].get("name", None)
    if not firewallName:
        return
    if config["server"].get("host_type", None) == "GCP":
        project = config["server"].get("gcp_project", None)
        if project:
            # Only support GCP at the moment
            try:
                import googleapiclient.discovery
                gcloud = googleapiclient.discovery.build("compute", "v1", cache_discovery=False)
                existingFW = gcloud.firewalls().get(project=project, firewall=firewallName).execute()
                patch = {'sourceRanges': [x for x in existingFW["sourceRanges"] if x != f"{tgtHost}/32"]}
                gcloud.firewalls().patch(project=project, firewall=firewallName, body=patch).execute()
            except Exception as e:
                # TODO: Log
                raise



def load_cluster_info(args):
    load_config(accessKey=args.accessKey)
    args.cluster_dir = g_baseDir / 'clusters' / f'cluster_{args.cluster_id}'
    if not args.cluster_dir.is_dir():
        raise FileExistsError(f"Cluster ID {args.cluster_id} does not exist")

    if 'cloud' not in args:
        for c in ["google"]:
            d = args.cluster_dir / 'terraform' / c
            if d.is_dir():
                args.cloud = c
                break
        else:
            raise FileExistsError(f"Unable to determine Cloud type of Cluster Dir {args.cluster_dir.as_posix()}")

    stateFile = args.cluster_dir / 'terraform' / args.cloud / 'terraform.tfstate'
    with stateFile.open('r') as statefp:
        state = json.load(statefp)
        args.cluster_ip = state["outputs"]["ManagementPublicIP"]["value"]
        args.cluster_name = state["outputs"]["cluster_id"]["value"]
        args.tf_state = state

    args.cluster_vars = _parse_tfvars(args.cluster_dir / 'terraform' / args.cloud / 'terraform.tfvars')



def rsync_dir(sourceDir, targetDir, args, log_dir, log_name='rsync_log', source_is_remote=False, rsync_opts=[]):
    """
    Requires 'args.cluster_dir' and 'args.cluster_ip'
    """
    ssh_key = args.cluster_dir / '.ssh' / 'id_rsa'
    ssh_args = f"ssh -i {ssh_key.as_posix()}"

    remote_str = f"citc@{args.cluster_ip}:"

    src_dir = f'{(remote_str if source_is_remote else "")}{sourceDir.as_posix()}/'
    tgt_dir = f'{(remote_str if not source_is_remote else "")}{targetDir.as_posix()}'

    rsync_ssh = f"ssh -i {ssh_key.as_posix()}"
    rsync_cmd = ["rsync", "-az", "--copy-unsafe-links", "-e", ssh_args]
    rsync_cmd.extend(rsync_opts)
    rsync_cmd.extend([src_dir, tgt_dir])

    newEnv = os.environ.copy()
    # Don't have terraform try to re-use any existing SSH agent
    # It has its own keys
    if "SSH_AUTH_SOCK" in newEnv:
        del newEnv["SSH_AUTH_SOCK"]


    try:
        subprocess.run(rsync_cmd, env=newEnv, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True)
    except subprocess.CalledProcessError as cpe:
        if cpe.stdout:
            with open(log_dir / f"{log_name}.stdout", 'wb') as log_out:
                log_out.write(cpe.stdout)
        if cpe.stderr:
            with open(log_dir / f"{log_name}.stderr", 'wb') as log_err:
                log_err.write(cpe.stderr)
        raise



def run_terraform(tgtDir, command, arguments=[], extraEnv={}):
    cmdline = ["terraform", command, "-no-color"]
    cmdline.extend(arguments)
    if command in ["apply", "destroy"]:
        cmdline.append("-auto-approve")
   
    log_out_fn = tgtDir / f"terraform_{command}_log.stdout"
    log_err_fn = tgtDir / f"terraform_{command}_log.stderr"

    newEnv = os.environ.copy()
    # Don't have terraform try to re-use any existing SSH agent
    # It has its own keys
    if "SSH_AUTH_SOCK" in newEnv:
        del newEnv["SSH_AUTH_SOCK"]
    newEnv.update(extraEnv)

    with log_out_fn.open('wb') as log_out:
        with log_err_fn.open('wb') as log_err:
            proc = subprocess.run(cmdline, cwd=tgtDir, env=newEnv,
                    stdout=log_out, stderr=log_err, check=True)

    return (log_out_fn, log_err_fn)


