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
"""Cluster Manager Backend for GHPCFE"""
import json
import logging
import uuid

from google.api_core.exceptions import AlreadyExists
from google.cloud import pubsub

from . import utils

# Note: We can't import Models here, because this module gets run as part of
# startup, and the Models haven't yet been created.
# Instead, import Models in the Callback Functions as appropriate

# pylint: disable=import-outside-toplevel

logger = logging.getLogger(__name__)

# Current design:
#  1 topic for our overall system
#  Subscription for FE is set with a filter of messages WITHOUT a "target"
#  attribute.  Subscriptions for Clusters each have a filter for a "target"
#  attribute that matches the cluster's ID.  (ideally, should rather be
#  something less guessable, like a hash or a unique key.)

# Message data should be json-encoded.
# Message attributes are used by the C2-infrastructure - data is for programmer
# use
# Messages should have the following attributes:
#   * target={subscription_name}  (or no target, to come to FE)
#   * command=('ping', 'sub_job', etc...)
# If no 'target' (aka, coming from the clusters:
#   * source={cluster_id} - Who sent it?

# Command with response callback
#
# Commands that require a response should encode a unique key as a message
# field ('ackid').
# When receiver finishes the command, they should then send an ACK with that
# same 'ackid', and any associated data.

_c2_callbackMap = {}


def c2_ping(message, source_id):
    # Expect source_id in the form of 'cluster_{id}'
    if "id" in message:
        pid = message["id"]
        logger.info(
            "Received PING id %s from cluster %s. Sending PONG", pid, source_id
        )
        _C2STATE.send_message("PONG", {"id": pid}, target=source_id)
    else:
        logger.info("Received anonymous PING from cluster %s", source_id)
    return True


def c2_pong(message, source_id):
    # Expect source_id in the form of 'cluster_{id}'
    if "id" in message:
        logger.info(
            "Received PONG id %s from cluster %s.", message["id"], source_id
        )
    else:
        logger.info("Received PONG from cluster %s", source_id)
    return True


# Difference between UPDATE and ACK:  ACK removes the callback, UPDATE leaves it
# in place
def cb_ack(message, source_id):
    from ..models import C2Callback

    ackid = message.get("ackid", None)
    logger.info("Received ACK to message %s from %s", ackid, source_id)
    if not ackid:
        logger.error("No ackid in ACK.  Ignoring")
        return True
    try:
        entry = C2Callback.objects.get(ackid=uuid.UUID(ackid))
        logger.info("Calling Callback registered for this ACK")
        cb = entry.callback
        entry.delete()
        cb(message)
    except C2Callback.DoesNotExist:
        logger.warning("No Callback registered for the ACK")
        pass

    return True


# Difference between UPDATE and ACK:  ACK removes the callback, UPDATE leaves it
# in place
def cb_update(message, source_id):
    from ..models import C2Callback

    ackid = message.get("ackid", None)
    if not ackid:
        logger.error("No ackid in UPDATE.  Ignoring")
        return True
    logger.info("Received UPDATE to message %s from %s", ackid, source_id)
    try:
        entry = C2Callback.objects.get(ackid=uuid.UUID(ackid))
        logger.info("Calling Callback registered for this UPDATE")
        cb = entry.callback
        cb(message)
    except C2Callback.DoesNotExist:
        logger.warning("No Callback registered for the UPDATE")
        pass

    return True


def cb_cluster_status(message, source_id):
    from ..models import Cluster

    try:
        cid = message["cluster_id"]
        if f"cluster_{cid}" != source_id:
            raise ValueError(
                "Message comes from {source_id}, but claims cluster {cid}. "
                "Ignoring."
            )

        cluster = Cluster.objects.get(pk=cid)
        logger.info(
            "Cluster Status message for %s: %s", cluster.id, message["message"]
        )
        new_status = message.get("status", None)
        if new_status:
            cluster.status = new_status
            cluster.save()
    # This logs the fall-through errors
    except Exception as ex:  # pylint: disable=broad-except
        logger.error("Cluster status callback error!", exc_info=ex)
    return True


def _c2_response_callback(message):
    logger.debug("Received message %s ", message)

    cmd = message.attributes.get("command", None)
    try:
        source = message.attributes.get("source", None)
        if not source:
            logger.error("Message had no Source ID")

        callback = _c2_callbackMap[cmd]
        if callback(json.loads(message.data), source_id=source):
            message.ack()
        else:
            message.nack()
        return
    except KeyError:
        if cmd:
            logger.error(
                'Message requests unknown command "%s".  Discarding', cmd
            )
        else:
            logger.error(
                "Message has no command associated with it. Discarding"
            )
    message.ack()


class _C2State:
    """Internal pubsub state management"""

    def __init__(self):
        self._pub_client = None
        self._sub_client = None
        self._streaming_pull_future = None
        self._project_id = None
        self._topic = None
        self._topic_path = None

    @property
    def sub_client(self):
        if not self._sub_client:
            self._sub_client = pubsub.SubscriberClient()
        return self._sub_client

    @property
    def pub_client(self):
        if not self._pub_client:
            self._pub_client = pubsub.PublisherClient()
        return self._pub_client

    def startup(self):
        conf = utils.load_config()
        self._project_id = conf["server"]["gcp_project"]
        self._topic = conf["server"]["c2_topic"]
        self._topic_path = self.pub_client.topic_path(
            self._project_id, self._topic
        )

        sub_path = self.get_or_create_subscription(
            "c2resp", filter_target=False
        )

        self._streaming_pull_future = self.sub_client.subscribe(
            sub_path, callback=_c2_response_callback
        )
        # TODO: Currently no clean shutdown method

    def get_subscription_path(self, sub_id):
        sub_id = f"{self._topic}-{sub_id}"
        return self.sub_client.subscription_path(self._project_id, sub_id)

    def get_or_create_subscription(
        self, sub_id, filter_target=True, service_account=None
    ):
        sub_path = self.get_subscription_path(sub_id)

        request = {"name": sub_path, "topic": self._topic_path}
        if filter_target:
            request["filter"] = f'attributes.target="{sub_id}"'
        else:
            request["filter"] = "NOT attributes:target"

        try:
            # Create subscription if it doesn't already exist
            self.sub_client.create_subscription(request=request)
            logger.info("PubSub Subscription %s created", sub_path)

            if service_account:
                self.setup_service_account(sub_id, service_account)

        except AlreadyExists:
            logger.info("PubSub Subscription %s already exists", sub_path)

        return sub_path

    def setup_service_account(self, sub_id, service_account):
        sub_path = self.get_subscription_path(sub_id)
        # Need to set 2 policies.  One on the subscription, to allow
        # access to subscribe one on the main topic, to allow
        # publication (c2 response)

        policy = self.sub_client.get_iam_policy(request={"resource": sub_path})
        policy.bindings.add(
            role="roles/pubsub.subscriber",
            members=[f"serviceAccount:{service_account}"],
        )
        policy = self.sub_client.set_iam_policy(
            request={"resource": sub_path, "policy": policy}
        )

        policy = self.pub_client.get_iam_policy(
            request={"resource": self._topic_path}
        )
        policy.bindings.add(
            role="roles/pubsub.publisher",
            members=[f"serviceAccount:{service_account}"],
        )
        policy = self.pub_client.set_iam_policy(
            request={"resource": self._topic_path, "policy": policy}
        )

    def delete_subscription(self, sub_id, service_account):
        sub_path = self.get_subscription_path(sub_id)
        self.sub_client.delete_subscription(request={"subscription": sub_path})
        if service_account:
            # TODO:  Remove IAM permission from topic
            # policy = self.pub_client.get_iam_policy(request={"resource":
            # sub_path}) policy.bindings.remove(role='roles/pubsub.publisher',
            # members=[f"serviceAccount:{service_account}"]) policy =
            # self.pub_client.set_iam_policy(request={"resource": sub_path,
            # "policy": policy})
            pass

    def send_message(self, command, message, target, extra_attrs=None):

        extra_attrs = extra_attrs if extra_attrs else {}
        # TODO: If we want loopback, need to make 'target' optional,
        # or change up our filters
        # TODO: Consider if we want to keep the futures or not
        self.pub_client.publish(
            self._topic_path,
            bytes(json.dumps(message), "utf-8"),
            target=target,
            command=command,
            **extra_attrs,
        )


_C2STATE = None


def get_cluster_sub_id(cluster_id):
    return f"cluster_{cluster_id}"


def get_cluster_subscription_path(cluster_id):
    return _C2STATE.get_subscription_path(get_cluster_sub_id(cluster_id))


def create_cluster_subscription(cluster_id):
    return _C2STATE.get_or_create_subscription(
        get_cluster_sub_id(cluster_id), filter_target=True
    )


def add_cluster_subscription_service_account(cluster_id, service_account):
    return _C2STATE.setup_service_account(
        get_cluster_sub_id(cluster_id), service_account
    )


def delete_cluster_subscription(cluster_id, service_account=None):
    return _C2STATE.delete_subscription(
        get_cluster_sub_id(cluster_id), service_account=service_account
    )


def get_topic_path():
    return _C2STATE._topic_path #pylint: disable=protected-access


def startup():
    global _C2STATE
    if _C2STATE:
        logger.error("ERROR:  C&C PubSub already started!")
        return

    _C2STATE = _C2State()
    _C2STATE.startup()
    # Difference between UPDATE and ACK:  ACK removes the callback, UPDATE
    # leaves it in place
    register_command("ACK", cb_ack)
    register_command("UPDATE", cb_update)
    register_command("PING", c2_ping)
    register_command("PONG", c2_pong)
    register_command("CLUSTER_STATUS", cb_cluster_status)


def send_command(cluster_id, cmd, data, on_response=None):
    if on_response:
        from ..models import C2Callback

        callback_entry = C2Callback(callback=on_response)
        callback_entry.save()
        data["ackid"] = str(callback_entry.ackid)
    _C2STATE.send_message(
        command=cmd, message=data, target=get_cluster_sub_id(cluster_id)
    )
    return data["ackid"]


def send_update(cluster_id, comm_id, data):
    # comm_id is result from `send_command()`
    data["ackid"] = comm_id
    _C2STATE.send_message(
        command="UPDATE", message=data, target=get_cluster_sub_id(cluster_id)
    )


def register_command(command_id, callback):
    _c2_callbackMap[command_id] = callback

def _cloud_build_logs_callback(message):
    import json
    try:
        raw_data = message.data.decode("utf-8")
        logger.debug("Received Pub/Sub message: %s", raw_data)
        log_entry = json.loads(raw_data)
        logger.debug("Parsed log entry: %s", log_entry)

        # Try to get the full build object from jsonPayload or protoPayload.
        build = log_entry.get("jsonPayload", {}).get("build")
        if not build:
            build = log_entry.get("protoPayload", {}).get("build")

        # If still no build object, use fallback: look for final step messages.
        if not build:
            build_id = log_entry.get("resource", {}).get("labels", {}).get("build_id")
            text = log_entry.get("textPayload", "").strip().upper()
            if text in ["DONE", "PUSH"]:
                status_str = "SUCCESS"
                logger.info("Interpreting textPayload '%s' as final success for build_id=%s", text, build_id)
            elif "FAIL" in text or "ERROR" in text or "CANCELLED" in text or "FAILED" in text:
                status_str = "FAILURE"
                logger.info("Interpreting textPayload '%s' as failure for build_id=%s", text, build_id)
            else:
                logger.debug("Ignoring non-final log for build_id=%s with textPayload: %s", build_id, text)
                message.ack()
                return
        else:
            build_id = build.get("id")
            status_str = build.get("status")
            logger.info("Extracted build object: build_id=%s, status=%s", build_id, status_str)

        logger.info("Processing Cloud Build log for build_id=%s with status=%s", build_id, status_str)

        # Only update if final status is reached.
        if status_str in ["SUCCESS", "FAILURE", "CANCELLED", "ERROR"]:
            from ..models import ContainerRegistry
            updated_count = 0
            for reg in ContainerRegistry.objects.all():
                modified = False
                for binfo in reg.build_info:
                    if binfo.get("build_id") == build_id:
                        logger.info("Before update for build_id=%s: current status=%s", build_id, binfo.get("status"))
                        if status_str == "SUCCESS":
                            binfo["status"] = "s"
                        elif status_str in ["FAILURE", "CANCELLED", "ERROR"]:
                            binfo["status"] = "f"
                        logger.info("After update for build_id=%s: new status=%s", build_id, binfo["status"])
                        modified = True
                if modified:
                    logger.info("Saving updated status for build_id=%s in registry ID %s", build_id, reg.id)
                    reg.save(update_fields=["build_info"])
                    updated_count += 1
            if updated_count:
                logger.info("Updated %d ContainerRegistry record(s) for build_id=%s", updated_count, build_id)
            else:
                logger.warning("No ContainerRegistry record updated for build_id=%s", build_id)
        else:
            logger.debug("Log for build_id=%s has non-final status (%s); no DB update.", build_id, status_str)

        message.ack()
        logger.debug("Message acknowledged for build_id=%s", build_id)

    except Exception as e:
        logger.exception("Error processing Cloud Build log entry: %s", e)
        message.nack()


def start_cloud_build_log_subscriber():
    conf = utils.load_config()
    project_id = conf["server"]["gcp_project"]
    deployment_name = conf["server"]["deployment_name"]
    subscription_id = f"{deployment_name}-build-logs-sub"

    subscriber = pubsub.SubscriberClient()
    subscription_path = subscriber.subscription_path(project_id, subscription_id)

    logger.info("Starting Cloud Build subscription: %s", subscription_path)
    subscriber.subscribe(subscription_path, callback=_cloud_build_logs_callback)
    logger.info("Cloud Build subscriber started.")
