#! /usr/bin/env python3
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


import daemon
import daemon.pidfile
import psutil
import subprocess as sp
import logging
import pymysql
import inspect
import socket
import os

from http.server import HTTPServer, BaseHTTPRequestHandler
from pathlib import Path


FILE_NAME = Path(__file__).stem
LOG_PATH = Path('/var/log/slurm/', FILE_NAME).with_suffix('.log')
LOCK_PATH = Path('/var/run/slurm/', FILE_NAME).with_suffix('.pid')
CONF_PATH = Path('/usr/local/etc/slurm/', "slurmdbd.conf")

HTTP_PORT = 8080 #HTTP port for healthcheck

hostname =os.environ.get("SLURMD_NODENAME", socket.gethostname())
if "controller0" in hostname:
    SERVICES_AND_PORTS = {
        "slurmctld": [6820, 6821, 6822, 6823, 6824, 6825, 6826, 6827, 6828, 6829, 6830], 
        "slurmdbd": [6819], 
        "slurmrestd": [6842, 8383, 8642],
    } #List of services to check. Format = service: TCP_port
else:
    SERVICES_AND_PORTS = {
        "slurmctld": [6820],
        "slurmrestd": [6842, 8383, 8642],
    }

# Logging
logging.basicConfig()
logger = logging.getLogger()
logger.setLevel(logging.INFO)

formatter = logging.Formatter('%(asctime)s - %(levelname)s - %(message)s')
file_handler = logging.FileHandler(LOG_PATH)
file_handler.setLevel(logging.INFO)
file_handler.setFormatter(formatter)
logger.addHandler(file_handler)


class Healthcheck:
    def __init__(self, services_and_ports):
        self.services_and_ports = services_and_ports

    def load_slurmdbd_config(self, config_path):
        try:
            with open(config_path) as f:
                config = [line.strip().split('=', 1) for line in f if not line.startswith("#") and "=" in line]
                config = dict(config)

                self.slurmdb_conf = {
                    "host": config["StorageHost"],
                    "user": config["StorageUser"],
                    "port": int(config["StoragePort"]),
                    "password": config["StoragePass"].strip("\""),
                    "database": config["StorageLoc"],
                }
        except Exception as e:
            raise Exception(f'Function error: {inspect.currentframe().f_back.f_code.co_name} - {e}')

    def service_check(self, service):
        is_active = sp.call(["systemctl", "is-active", service]) == 0
        logger.info(f'{service} is active') if is_active else logger.error(f'{service} is not active')
        return is_active

    def process_check(self, service):
        """
        Vérifie si un service/processus est en cours d'exécution en cherchant
        dans les lignes de commande de tous les processus.
        """
        process = None
        
        for p in psutil.process_iter(['pid', 'name', 'status', 'cmdline']):
            try:
                # Récupère la ligne de commande complète
                cmdline = p.info['cmdline']
                process_name = p.info['name']
                
                # Vérifie si le service est dans le nom du processus OU dans la ligne de commande
                if (service == process_name or 
                    (cmdline and any(service in part for part in cmdline))):
                    process = p
                    break
                    
            except (psutil.NoSuchProcess, psutil.AccessDenied, psutil.ZombieProcess):
                # Ignore les processus qui ne sont plus accessibles
                continue
        
        logger.info(f"process: {process}")
        
        if process:
            try:
                # Vérifie le statut du processus trouvé
                process_ok = process.info['status'] in ["sleeping", "running"]
                logger.info(f'{service} process is healthy (PID: {process.pid})') if process_ok else logger.error(f'{service} process unhealthy (status: {process.info["status"]})')
                return process_ok
            except (psutil.NoSuchProcess, psutil.AccessDenied):
                logger.error(f'{service} process found but became inaccessible')
                return False
        else:
            logger.error(f'{service} process not found')
            return False

    def tcp_check(self, port, service=""):
        tcp_ok = any(conn.laddr[1] == port for conn in psutil.net_connections(kind='tcp'))
        logger.info(f'{service} - PORT {port} is open') if tcp_ok else logger.error(f'{service} - PORT {port} is closed')
        return tcp_ok

    def sql_check(self):
        try:
            connection = pymysql.connect(**self.slurmdb_conf)
            logger.info("Connection to MySQL was successful")
        except pymysql.MySQLError as connection_error:
            logger.error(f"Error connecting to database: {connection_error}")
            return False

        try:
            with connection.cursor() as cursor:
                cursor.execute("SELECT 1")
                result = cursor.fetchone()
                if result == (1,):
                    logger.info("Query was successful")
                    return True
                else:
                    logger.error("Unexpected result from database query")
                    return False
        except pymysql.MySQLError as query_error:
            logger.error(f"Error executing query: {query_error}")
            return False
        finally:
            connection.close()
    
    def slurmctld_check(self):
        logger.info("I am in slurmctld_check function.....")
        service_name = "slurmctld"
        service_active = self.service_check(service_name)
        process_ok = self.process_check(service_name)
        slurm_ping = sp.run(["scontrol", "ping"], check=True).returncode == 0
        tcp_ok = all([self.tcp_check(port, service_name) for port in self.services_and_ports[service_name]])
        return all([service_active, slurm_ping, tcp_ok, process_ok])

    def slurmdbd_check(self):
        logger.info("I am in slurmdbd_check function.....")
        service_name = "slurmdbd"
        service_active = self.service_check(service_name)
        process_ok = self.process_check(service_name)
        tcp_ok = all([self.tcp_check(port, service_name) for port in self.services_and_ports[service_name]])
        sql_ok = self.sql_check()
        return all([service_active, tcp_ok, sql_ok, process_ok])
    
    def slurmrestd_check(self):
        logger.info("I am in slurmrestd_check function.....")
        service_name = "slurmrestd"
        service_active = self.service_check(service_name)
        process_ok = self.process_check(service_name)
        tcp_ok = all([self.tcp_check(port, service_name) for port in self.services_and_ports[service_name]])
        return all([service_active, tcp_ok, process_ok])

    def healthcheck(self, req_path):
        logger.info("I am in healthcheck function.....")
        checks = {
            "/sql": self.sql_check,
            "/slurmctld": self.slurmctld_check,
            "/slurmdbd": self.slurmdbd_check,
            "/slurmrestd": self.slurmrestd_check,
        }
        if req_path in checks:
            return checks[req_path]()
        elif "/service/" in req_path:
            service = req_path.split("/")[-1]
            if service in self.services_and_ports.keys():
                return self.service_check(service)
            logger.error(f'Service "{service}" not recognized')
        elif "/port/" in req_path:
            port = int(req_path.split("/")[-1])
            return self.tcp_check(port)
        else:
            return all([
                self.slurmctld_check() if "slurmctld" in self.services_and_ports else True,
                self.slurmdbd_check() if "slurmdbd" in self.services_and_ports else True,
                self.slurmrestd_check() if "slurmrestd" in self.services_and_ports else True,
            ])
            

class RequestHandler(BaseHTTPRequestHandler):
    """Handle simple HTTP Request"
    """
    def do_GET(self):
        logger.info("I am in do_GET function.....")
        return_code = main_program(self.path)
        self.send_response(return_code)
        self.send_header("Content-type", "text/html")
        self.end_headers()
        self.wfile.write(f"GET request for {self.path} - Return Code: {return_code}".encode("utf-8"))
                
            
def run_server():
    logger.info("I am in run_server function.....")
    server_addr = ("", HTTP_PORT)
    print(server_addr)
    httpd = HTTPServer(server_addr, RequestHandler)
    httpd.serve_forever()
            

def main_program(req_path):
    logger.info("I am in main function.....")
    try:
        resp = hc.healthcheck(req_path)
        logger.info(f"resp:{resp} ")
        return 200 if resp else 400
    except Exception as error:
        logger.critical(error)
        return 500


with daemon.DaemonContext(
                files_preserve = [file_handler.stream,], #Keeps logging after daemonize
                pidfile=daemon.pidfile.PIDLockFile(LOCK_PATH), #Lock so it won't run twice
        ):
    logger.info("slurmhcd daemon started")
    hc = Healthcheck(SERVICES_AND_PORTS)
    logger.info("Debugging log configuration")
    hc.load_slurmdbd_config(CONF_PATH)
    run_server()
