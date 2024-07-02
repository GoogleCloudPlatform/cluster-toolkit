import logging
import sys
import slurm_gcp_plugins.utils as sgp_utils

# Allows setting a specific max_hop for jobs
#
# To enable:
# * add this directory to the slurm-gcp plugin path (usually /slurm/scripts/slurm-gcp-plugins)
# * add the following to the slurm-gcp config (usually /slurm/scripts/config.yaml):
#
# enable_slurm_gcp_plugins:
#  <possibly other plugins>
#  max_hops:
#    max_hops: <hops>
#
#
# Where <hps> can be either of 1,2,3 (in increasing order of distance)
# If no max_hops is provided but the plugins is still enabled the default level is 3


def pre_placement_group_insert(*pos_args, **keyword_args):
    logging.info("Trying to enable max hop")
    # Avoid circular import (util imports the plugins)
    if "util" in sys.modules:
        logging.info("Setting compute service version to beta")
        sys.modules["util"].compute = sys.modules["util"].compute_service(
            version="beta"
        )
        max_distance = sgp_utils.get_plugin_setting(
            plugin="max_hops",
            setting="max_hops",
            job=get_job_from_placement_group_name(keyword_args["pg_name"]),
            lkp=keyword_args["lkp"],
            default=3,
        )
        logging.debug(f"Setting max hop for placement policy to {max_distance}")
        keyword_args["request_body"]["groupPlacementPolicy"][
            "collocation="
        ] = "COLLOCATED"
        keyword_args["request_body"]["groupPlacementPolicy"][
            "maxDistance"
        ] = max_distance
    else:
        logging.error(
            "max_hops can not be set (slurm_gcp util.py must be imported by the caller of the plugin callback)"
        )


__all__ = [
    "pre_placement_group_insert",
]


# This should be replaced if the job id becomes available in the context of this plugin hook
def get_job_from_placement_group_name(pg_name):
    # f"{cfg.slurm_cluster_name}-{partition_name}-{job_id}-{i}"

    return pg_name.split("-")[2]
