# Copyright 2024 "Google LLC"
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

import logging

instance_information_fields = ["resourceStatus", "id"]


def register_instance_information_fields(*pos_args, **keyword_args):
    logging.debug("register_instance_information_fields called from test_plugin")
    keyword_args["instance_information_fields"].extend(instance_information_fields)


def post_main_resume_nodes(*pos_args, **keyword_args):
    logging.debug("post_main_resume_nodes called from test_plugin")
    for node in keyword_args["nodelist"]:
        logging.info(
            (
                "test_plugin:"
                + f"nodename:{node} "
                + f"instance_id:{keyword_args['lkp'].instance(node)['id']} "
                + f"physicalHost:{keyword_args['lkp'].instance(node)['resourceStatus']['physicalHost']}"
            )
        )


__all__ = [
    "register_instance_information_fields",
    "post_main_resume_nodes",
]
