import os
import re
import subprocess
from typing import List
from optparse import OptionParser

class NodeMaintenance:
    '''
    Class to keep track of project, zones, and search terms for nodes, as well as results from gcloud queries
    '''
    def __init__(self, project, zones, regex):
        self.project = project
        res = subprocess.run("gcloud projects describe {}".format(self.project), shell=True, capture_output=True, text=True)
        if res.returncode != 0:
            raise Exception("{} does not exist or you may not have permission to access it".format(self.project))
        
        self.zones = None
        if zones:
            self.zones = zones.split(",")
            for zone in self.zones:
                res = subprocess.run("gcloud compute zones describe {}".format(zone), shell=True, capture_output=True, text=True)
                if res.returncode != 0:
                    raise Exception("{} does not exist".format(zone))
        
        self.regex = None
        self.screen_reader = False
        res = subprocess.run("gcloud config get accessibility/screen_reader", 
                             shell=True, capture_output=True, text=True)
        if res.returncode != 0:
            raise Exception("Error getting accessibility/screen_reader information")
        if res.stdout.split("\n")[0] == "True":
            self.screen_reader = True
        if regex:
            self.regex = re.compile(regex)
        self.maint_nodes = None
        self.upc_maint = None
        self.get_maint_nodes()
        self.get_upcoming_maint()

    # Needs to be used after the command is fully created
    def remove_accessibility(self, cmd: str) -> str:
        return "gcloud config set accessibility/screen_reader False && " \
               "{} && gcloud config set accessibility/screen_reader True".format(cmd)
    
    def get_maint_nodes(self) -> List[str]:
        cmd = "gcloud alpha compute instances list --project={}".format(self.project)
        if self.zones is not None:
            cmd += " --zones=" + ",".join(self.zones)
        cmd += " --filter=scheduling.maintenanceInterval:PERIODIC --format='table(name)'"

        if self.screen_reader:
            cmd = self.remove_accessibility(cmd)

        res = subprocess.run(cmd, shell=True, capture_output=True, text=True)
        if res.returncode == 0:
            self.maint_nodes = res.stdout.split("\n")[1:-1]
            if self.regex:
                self.maint_nodes = list(filter(self.regex.match, names))
        else:
            raise Exception("Error getting VMs that have scheduled maintenance:\n" + res.stderr)

    def get_upcoming_maint(self) -> List[str]:
        cmd = "gcloud alpha compute instances list --project={} --filter='upcomingMaintenance:*".format(self.project)
        if self.zones is not None:
            # NOTE: This link will likely need to be updated once this feature moves out of alpha
            # Used instead of --zones as that breaks the filter feature
            links = ["\"https://www.googleapis.com/compute/alpha/projects/{}/zones/{}\"".format(self.project, x) for x in self.zones]
            cmd += " AND (zone=" + " OR ".join(links) + ")"
        cmd +=  "' --format='table(name, upcomingMaintenance.startTimeWindow.earliest:label=EARLIEST_START," \
                " upcomingMaintenance.startTimeWindow.latest:label=LATEST_START," \
                " upcomingMaintenance.canReschedule, upcomingMaintenance.type)'"

        if self.screen_reader:
            cmd = self.remove_accessibility(cmd)

        res = subprocess.run(cmd, shell=True, capture_output=True, text=True)
        if res.returncode == 0:
            self.upc_maint = [x.split() for x in res.stdout.split("\n")[1:-1]]
            if self.regex:
                self.upc_maint = list(filter(lambda x: self.regex.match(x[0]), table))
        else:
            raise Exception("Error getting upcoming maintenance list:\n" + res.stderr)

    def start_maint(self, project: str, zones: str):
        pass

    def print_maintenance_nodes(self):
        if not self.maint_nodes or len(self.maint_nodes) == 0:
            print("No nodes with periodic maintenance\n")
            return

        print("Nodes with PERIODIC maintenance")
        for x in self.maint_nodes:
            print(x)
        print()

    def print_upcoming_maintenance(self):
        if not self.upc_maint or len(self.upc_maint) == 0:
            print("No upcoming maintenance\n")
            return 

        print("Upcoming maintenance:")
        row_format ="{:30}" * (len(self.upc_maint[0]))
        print(row_format.format(*["Name", "Earliest Start", "Latest Start", "Can Reschedule", "Maintenance Type"]))
        for row in self.upc_maint:
            print(row_format.format(*row))
        print()

if __name__ == "__main__":
    parser = OptionParser()
    parser.add_option("-p", "--project", dest="project",
                      help="GCP project id")
    parser.add_option("-z", "--zones", dest="zones",
                      help="deployment zones (comma delimited, no spaces)")
    parser.add_option("-n", "--node-regex", dest="node_regex",
                      help="Regular expression search for nodes")
    # parser.add_option("-m", "--check-maintenance", action="store_true", dest="check_maintenance",
    #                   help="Get nodes that have periodic maintenance setup (default)")
    # parser.add_option("-u", "--check-upcoming", action="store_true", dest="check_upcoming",
    #                   help="Get nodes that have upcoming maintenance")

    (options, args) = parser.parse_args()

    if not options.project:
        raise Exception("Project must be specified")

    maint = NodeMaintenance(options.project, options.zones, options.node_regex)
    
    maint.print_maintenance_nodes()
    maint.print_upcoming_maintenance()
