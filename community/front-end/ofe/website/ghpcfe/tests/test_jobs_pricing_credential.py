# Copyright 2026 Google LLC
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

"""The job pages look up the node price with the cluster's credential.

get_instance_pricing acts as a Credential (a key or an impersonated
account), so the job create and rerun pages pass the cluster's Credential
itself, never its stored detail. Offline: the pricing lookup is replaced at
its module boundary, cloud_info.get_instance_pricing.
"""

from unittest import mock

from django.test import TestCase
from django.urls import reverse

from ghpcfe.models import Application, Credential, Job
from ghpcfe.tests.test_blueprint_identities import (
    make_cluster,
    make_partition,
    make_world,
)

NODE_PRICE = 1.25


class JobPricingCredentialTests(TestCase):
    """Job create and rerun price the job as the cluster's credential"""

    def setUp(self):
        world = make_world()
        self.user, self.credential, _ = world
        self.user.quota_type = "u"
        self.user.save()
        self.client.force_login(self.user)
        self.cluster = make_cluster(world, 101, "alpha", "0a1b2c3d")
        self.partition = make_partition(self.cluster, 201, "batch")
        self.application = Application.objects.create(
            name="hello", cluster=self.cluster, status="r"
        )
        patcher = mock.patch(
            "ghpcfe.cluster_manager.cloud_info.get_instance_pricing",
            return_value=NODE_PRICE,
        )
        self.addCleanup(patcher.stop)
        self.pricing = patcher.start()

    def job_fields(self, name):
        return {
            "name": name,
            "application": self.application.pk,
            "cluster": self.cluster.pk,
            "partition": self.partition.pk,
            "number_of_nodes": 2,
            "ranks_per_node": 4,
            "threads_per_rank": 1,
            "wall_clock_time_limit": 60,
            "run_script": "#!/bin/bash\necho hello",
            "cleanup_choice": "n",
            "input_data": "",
            "result_data": "",
            "benchmark": "",
        }

    def assert_priced_as_the_credential(self, response, name):
        job = Job.objects.get(name=name)
        self.assertRedirects(
            response,
            reverse("backend-job-run", kwargs={"pk": job.pk}),
            fetch_redirect_response=False,
        )
        self.pricing.assert_called_once()
        args = self.pricing.call_args.args
        # The cluster's Credential row, not its detail text (a model
        # instance equals another of the same model and primary key)
        self.assertEqual(self.cluster.cloud_credential_id, self.credential.pk)
        self.assertIsInstance(args[1], Credential)
        self.assertEqual(
            args,
            (
                "GCP",
                self.credential,
                self.cluster.cloud_region,
                self.cluster.cloud_zone,
                self.partition.machine_type,
            ),
        )
        self.assertEqual(float(job.node_price), NODE_PRICE)

    def test_create_prices_with_the_cluster_credential(self):
        response = self.client.post(
            reverse(
                "job-create-2",
                kwargs={"app": self.application.pk, "cluster": self.cluster.pk},
            ),
            self.job_fields("first-run"),
        )
        self.assert_priced_as_the_credential(response, "first-run")

    def test_rerun_prices_with_the_cluster_credential(self):
        earlier = Job.objects.create(
            name="earlier",
            application=self.application,
            cluster=self.cluster,
            partition=self.partition,
            user=self.user,
            number_of_nodes=1,
            ranks_per_node=1,
            run_script="#!/bin/bash\necho hello",
        )
        response = self.client.post(
            reverse("job-rerun", kwargs={"job": earlier.pk}),
            self.job_fields("second-run"),
        )
        self.assert_priced_as_the_credential(response, "second-run")
