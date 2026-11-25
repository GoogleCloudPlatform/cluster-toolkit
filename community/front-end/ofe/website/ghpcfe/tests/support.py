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

"""Test-process stubs and guards, installed by the test settings.

Importing this module:

* installs stand-in ``spack`` modules, because ``cluster_manager/spack.py``
  imports Spack from a checkout that only exists on the deployed server, and
  the URLconf (loaded by the system checks) imports it through the views;
* makes every Pub/Sub client constructor raise, so no test can open a
  connection to the C2 topic or subscriptions.

It must not import Django models: it runs before the app registry is ready.
"""

import sys
import types

from google.cloud import pubsub_v1
from google.cloud.pubsub_v1.publisher import client as publisher_client
from google.cloud.pubsub_v1.subscriber import client as subscriber_client
import google.pubsub_v1 as pubsub_gapic
from google.pubsub_v1.services.publisher.transports import base as pub_transport
from google.pubsub_v1.services.subscriber.transports import base as sub_transport
from google.pubsub_v1.services.schema_service.transports import base as schema_transport


def _install_spack_stub():
    """Minimal spack.main, spack.repo and spack.version with no packages"""

    class _RepoPath:
        def get_pkg_class(self, name):
            raise KeyError(name)

    class _VersionList(list):
        def preferred(self):
            return sorted(self)[-1] if self else None

    spack = types.ModuleType("spack")
    spack.__path__ = []
    spack_main = types.ModuleType("spack.main")
    spack_repo = types.ModuleType("spack.repo")
    spack_repo.all_package_names = lambda: []
    spack_repo.PATH = _RepoPath()
    spack_version = types.ModuleType("spack.version")
    spack_version.VersionList = _VersionList

    spack.main = spack_main
    spack.repo = spack_repo
    spack.version = spack_version
    sys.modules["spack"] = spack
    sys.modules["spack.main"] = spack_main
    sys.modules["spack.repo"] = spack_repo
    sys.modules["spack.version"] = spack_version


# Every Pub/Sub client and transport class an OFE code path could construct,
# directly or through the higher-level clients.
PUBSUB_CLASSES = (
    pubsub_v1.PublisherClient,
    pubsub_v1.SubscriberClient,
    publisher_client.Client,
    subscriber_client.Client,
    pubsub_gapic.PublisherClient,
    pubsub_gapic.SubscriberClient,
    pubsub_gapic.SchemaServiceClient,
    pubsub_gapic.PublisherAsyncClient,
    pubsub_gapic.SubscriberAsyncClient,
    pubsub_gapic.SchemaServiceAsyncClient,
    pub_transport.PublisherTransport,
    sub_transport.SubscriberTransport,
    schema_transport.SchemaServiceTransport,
)

# Names of the classes whose construction was attempted (and refused)
blocked_pubsub_clients = []


class PubSubBlockedError(RuntimeError):
    """Raised when test code tries to construct a Pub/Sub client"""


def _block_pubsub():
    def make_init(cls_name):
        def _init(self, *args, **kwargs):  # pylint: disable=unused-argument
            blocked_pubsub_clients.append(cls_name)
            raise PubSubBlockedError(
                f"{cls_name} cannot be constructed in the test suite"
            )

        return _init

    for cls in PUBSUB_CLASSES:
        cls.__init__ = make_init(f"{cls.__module__}.{cls.__qualname__}")


def create_google_social_app():
    """Create the Google SocialApp on site 1, as custom_setup_command does.

    The login page renders ``provider_login_url 'google'``, which fails
    without it.
    """
    # Models can only be imported once the app registry is ready
    # pylint: disable=import-outside-toplevel
    from allauth.socialaccount.models import SocialApp
    from django.contrib.sites.models import Site

    socialapp = SocialApp.objects.create(
        provider="google",
        name="Google API",
        client_id="test-client-id",
        secret="test-secret",
    )
    socialapp.sites.add(Site.objects.get(pk=1))
    return socialapp


_install_spack_stub()
_block_pubsub()
