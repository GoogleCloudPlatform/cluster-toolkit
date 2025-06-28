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


def cb_cluster_status(message, source_id=None):
    """Handle cluster status updates"""
    # Import here to avoid circular imports
    from ..models import Cluster

    cluster_id = message.get("cluster_id")
    status = message.get("status")

    if cluster_id and status:
        try:
            cluster = Cluster.objects.get(id=cluster_id)
            cluster.status = status
            cluster.save()
            logger.info(f"Updated cluster {cluster_id} status to {status}")
        except Cluster.DoesNotExist:
            logger.warning(f"Cluster {cluster_id} not found in database")
        except Exception as e:
            logger.error(f"Error updating cluster {cluster_id} status: {e}")

    return True


def _extract_number(val, default=1):
    """Extract numeric value from various formats (dict, list, or direct value)"""
    if isinstance(val, dict) and 'number' in val:
        return val['number']
    elif isinstance(val, list):
        return _extract_number(val[0], default) if val else default
    elif isinstance(val, (int, float)):
        return val
    return default


def _normalize_slurm_status(status_raw):
    """Normalize SLURM status to a string, handling list format"""
    if isinstance(status_raw, list) and len(status_raw) > 0:
        logger.debug(f"slurm_status was a list, using first element: {status_raw[0]}")
        primary_status = status_raw[0]
        additional_states = status_raw[1:] if len(status_raw) > 1 else []
        return primary_status, additional_states
    elif isinstance(status_raw, str):
        return status_raw, []
    else:
        return str(status_raw) if status_raw is not None else "", []


def _is_job_successful(slurm_status, exit_code):
    """Determine if a job was successful based on SLURM status and exit code"""
    # For jobs created by OFE applications, we don't want to override their status
    # These jobs are handled by the existing application logic

    # If status is explicitly FAILED or CANCELLED, it's a failure
    if slurm_status in ['FAILED', 'CANCELLED']:
        return False

    # If status is COMPLETED, check the exit code
    if slurm_status == 'COMPLETED':
        if exit_code is None:
            # No exit code available, assume success (for backward compatibility)
            return True

        # Parse exit code - can be various formats
        try:
            # First, try to parse as JSON if it's a string
            if isinstance(exit_code, str):
                try:
                    import json
                    exit_code = json.loads(exit_code)
                except (ValueError, TypeError):
                    pass  # Not JSON, treat as regular string

            if isinstance(exit_code, dict):
                # Complex JSON format from SLURM: {'return_code': {'number': 1}, 'signal': {'id': {'number': 0}}}
                logger.debug(f"Parsing dict exit code: {exit_code}")
                if 'return_code' in exit_code and isinstance(exit_code['return_code'], dict):
                    return_code_data = exit_code['return_code']
                    if 'number' in return_code_data:
                        exit_code_num = return_code_data['number']
                        logger.debug(f"Extracted exit code number: {exit_code_num}")
                        result = exit_code_num == 0
                        logger.debug(f"Exit code {exit_code_num} == 0: {result}")
                        return result
                # Fallback: check if there's a direct 'number' field
                elif 'number' in exit_code:
                    exit_code_num = exit_code['number']
                    logger.debug(f"Extracted direct number: {exit_code_num}")
                    result = exit_code_num == 0
                    logger.debug(f"Direct exit code {exit_code_num} == 0: {result}")
                    return result
            elif isinstance(exit_code, str):
                logger.debug(f"Parsing string exit code: {exit_code}")
                if ':' in exit_code:
                    signal, code = exit_code.split(':', 1)
                    exit_code_num = int(code)
                else:
                    exit_code_num = int(exit_code)
                logger.debug(f"Extracted exit code number: {exit_code_num}")
                result = exit_code_num == 0
                logger.debug(f"String exit code {exit_code_num} == 0: {result}")
                return result
            elif isinstance(exit_code, (int, float)):
                logger.debug(f"Parsing numeric exit code: {exit_code}")
                result = int(exit_code) == 0
                logger.debug(f"Numeric exit code {exit_code} == 0: {result}")
                return result
            else:
                # Unknown format, assume success for backward compatibility
                logger.debug(f"Unknown exit code format: {exit_code}, assuming success")
                return True
        except (ValueError, TypeError, KeyError) as e:
            # If we can't parse the exit code, assume success (for backward compatibility)
            logger.debug(f"Could not parse exit code '{exit_code}': {e}, assuming success")
            return True

    # For other statuses (RUNNING, PENDING, etc.), not yet completed
    return None


def cb_slurm_job_update(message, source_id=None):
    """Handle SLURM job updates"""
    # Import here to avoid circular imports
    from ..models import Job, Cluster
    from django.db import transaction

    data = message.get("data", {})
    cluster_id = message.get("cluster_id")

    if not cluster_id:
        logger.warning("Received SLURM job update without cluster ID")
        return True

    try:
        cluster = Cluster.objects.get(id=cluster_id, status='r')
    except Cluster.DoesNotExist:
        logger.warning(f"Cluster {cluster_id} not found or not ready")
        return True

    slurm_jobid = data.get('slurm_jobid')
    if not slurm_jobid:
        logger.warning("Received SLURM job update without job ID")
        return True

    # Normalize slurm_jobid if it's a list
    if isinstance(slurm_jobid, list):
        slurm_jobid = slurm_jobid[0] if slurm_jobid else None
        logger.debug(f"slurm_jobid was a list, using first element: {slurm_jobid}")

    try:
        # Try to find existing job
        job = Job.objects.filter(slurm_jobid=slurm_jobid).first()

        if job:
            # Update existing job
            updates = {}

            # Parse timestamps
            if data.get('slurm_start_time'):
                start_time = _parse_slurm_timestamp(data['slurm_start_time'])
                if start_time:
                    updates['slurm_start_time'] = start_time

            if data.get('slurm_end_time'):
                end_time = _parse_slurm_timestamp(data['slurm_end_time'])
                if end_time:
                    updates['slurm_end_time'] = end_time
                    # Calculate runtime if we have both start and end times
                    if job.slurm_start_time:
                        try:
                            # Ensure both times are timezone-aware for comparison
                            start_time_for_calc = job.slurm_start_time
                            if start_time_for_calc.tzinfo is None:
                                from django.utils import timezone
                                start_time_for_calc = timezone.make_aware(start_time_for_calc)

                            end_time_for_calc = end_time
                            if end_time_for_calc.tzinfo is None:
                                from django.utils import timezone
                                end_time_for_calc = timezone.make_aware(end_time_for_calc)

                            runtime = (end_time_for_calc - start_time_for_calc).total_seconds()
                            updates['runtime'] = runtime
                        except Exception as e:
                            logger.debug(f"Could not calculate runtime for job {slurm_jobid}: {e}")
                            # Don't fail the entire update if runtime calculation fails

            # Handle slurm_status
            slurm_status, additional_states = _normalize_slurm_status(data.get('slurm_status', ''))
            if slurm_status:
                updates['slurm_status'] = slurm_status

            # Store additional states if any
            if additional_states:
                updates['slurm_additional_states'] = additional_states
                logger.debug(f"Storing additional states for job {slurm_jobid}: {additional_states}")

            # Extract exit code for job success determination
            exit_code = data.get('exit_code')
            if exit_code:
                logger.debug(f"Raw exit code from data: {exit_code} (type: {type(exit_code)})")
                updates['slurm_exit_code'] = exit_code

            # Extract numeric values using helper function
            nodes_allocated = _extract_number(data.get('nodes_allocated'), None)
            if nodes_allocated is not None:
                updates['number_of_nodes'] = nodes_allocated

            ntasks_per_node = _extract_number(data.get('ntasks_per_node'), None)
            if ntasks_per_node is not None:
                updates['ranks_per_node'] = ntasks_per_node

            cpus_per_task = _extract_number(data.get('cpus_per_task'), None)
            if cpus_per_task is not None:
                updates['threads_per_rank'] = cpus_per_task

            time_limit = _extract_number(data.get('time_limit'), None)
            if time_limit is not None:
                updates['wall_clock_time_limit'] = time_limit // 60  # Convert to minutes

            if updates:
                # Store old values for comparison
                old_slurm_status = getattr(job, 'slurm_status', None)
                old_start_time = getattr(job, 'slurm_start_time', None)
                old_end_time = getattr(job, 'slurm_end_time', None)

                # Protect cancelled jobs from being overwritten with completed status
                if old_slurm_status == 'CANCELLED' and 'slurm_status' in updates:
                    new_slurm_status = updates['slurm_status']
                    if new_slurm_status == 'COMPLETED':
                        logger.info(f'Job {job.id} (SLURM {slurm_jobid}) is already CANCELLED, preventing overwrite to COMPLETED')
                        # Remove the slurm_status update to preserve cancelled status
                        del updates['slurm_status']
                        # Also remove slurm_end_time if it would cause completion detection
                        if 'slurm_end_time' in updates:
                            logger.info(f'Job {job.id} (SLURM {slurm_jobid}) is cancelled, preserving original end time')
                            del updates['slurm_end_time']

                with transaction.atomic():
                    for field, value in updates.items():
                        setattr(job, field, value)
                    job.save(update_fields=list(updates.keys()))

                # Log only significant updates, not every field
                significant_fields = ['slurm_status', 'slurm_start_time', 'slurm_end_time', 'status']
                significant_updates = {k: v for k, v in updates.items() if k in significant_fields}
                if significant_updates:
                    # Check if this is a meaningful status change
                    status_changed = 'slurm_status' in updates and updates['slurm_status'] != old_slurm_status
                    time_changed = ('slurm_start_time' in updates and updates['slurm_start_time'] != old_start_time) or \
                                 ('slurm_end_time' in updates and updates['slurm_end_time'] != old_end_time)

                    if status_changed or time_changed:
                        logger.info(f'Updated job {job.id} (SLURM {slurm_jobid}): {significant_updates}')
                    else:
                        logger.debug(f'Updated job {job.id} (SLURM {slurm_jobid}): {significant_updates}')

                # Special handling for completed jobs
                if slurm_status in ['COMPLETED', 'FAILED', 'CANCELLED']:
                    # For cancelled jobs, use the stored status to prevent incorrect mapping
                    status_for_mapping = job.slurm_status if job.slurm_status == 'CANCELLED' else slurm_status
                    # Determine OFE status based on SLURM status and exit code
                    new_ofe_status = _map_slurm_status(status_for_mapping, exit_code)
                    logger.info(f'Job {job.id} (SLURM {slurm_jobid}) has SLURM status {status_for_mapping}, mapping to OFE status {new_ofe_status}')
                    if job.status != new_ofe_status:
                        job.status = new_ofe_status
                        job.save(update_fields=['status'])
                        logger.info(f'Updated OFE status for job {job.id} to {job.status} based on exit code and SLURM status')
                    else:
                        logger.debug(f'Job {job.id} already has OFE status {job.status}, not updating')

                # Additional completion detection: if job has end time but no specific status, determine completion
                elif job.slurm_end_time and job.slurm_end_time > (job.slurm_start_time or 0):
                    # Only mark as completed if we don't have a specific SLURM status that indicates error
                    if job.status not in ['c', 'e'] and slurm_status not in ['FAILED', 'CANCELLED', 'TIMEOUT', 'PREEMPTED']:
                        logger.info(f'Job {job.id} (SLURM {slurm_jobid}) has end time, marking as completed')
                        job.status = 'c'
                        job.save(update_fields=['status'])
                        logger.info(f'Updated OFE status for job {job.id} to completed based on end time')
                    elif slurm_status in ['FAILED', 'CANCELLED', 'TIMEOUT', 'PREEMPTED']:
                        # If SLURM status indicates error, mark as error
                        if job.status != 'e':
                            logger.info(f'Job {job.id} (SLURM {slurm_jobid}) has end time and error status {slurm_status}, marking as error')
                            job.status = 'e'
                            job.save(update_fields=['status'])
                            logger.info(f'Updated OFE status for job {job.id} to error based on SLURM status {slurm_status}')

                    # Also update the slurm_status field if it's None or empty, but preserve cancelled status
                    if not job.slurm_status or job.slurm_status == '':
                        # Don't overwrite cancelled status with completed
                        if slurm_status not in ['FAILED', 'CANCELLED', 'TIMEOUT', 'PREEMPTED']:
                            job.slurm_status = 'COMPLETED'
                            job.save(update_fields=['slurm_status'])
                            logger.info(f'Updated slurm_status for job {job.id} to COMPLETED')
                        else:
                            # Preserve the error status
                            job.slurm_status = slurm_status
                            job.save(update_fields=['slurm_status'])
                            logger.info(f'Updated slurm_status for job {job.id} to {slurm_status} (preserving error status)')

                # Handle case where slurm_status is None but we have completion info
                elif slurm_status is None and job.slurm_end_time:
                    # If we have an end time but no status, assume completed (but not if we already have error status)
                    if job.status not in ['c', 'e']:
                        job.status = 'c'
                        job.save(update_fields=['status'])
                        logger.info(f'Updated OFE status for job {job.id} to completed (status was None)')

                    # Update slurm_status to COMPLETED, but only if we don't already have an error status
                    if job.slurm_status not in ['FAILED', 'CANCELLED', 'TIMEOUT', 'PREEMPTED']:
                        job.slurm_status = 'COMPLETED'
                        job.save(update_fields=['slurm_status'])
                        logger.info(f'Updated slurm_status for job {job.id} to COMPLETED (was None)')
                    else:
                        logger.debug(f'Preserving existing error status {job.slurm_status} for job {job.id}')
        else:
            # Create new external job
            _create_external_job(data, cluster)

    except Exception as e:
        logger.error(f'Error processing SLURM job update for job {slurm_jobid}: {e}')
        import traceback
        logger.debug(f'Traceback: {traceback.format_exc()}')

    return True


def _parse_slurm_timestamp(timestamp_data):
    """Parse SLURM timestamp data which can be in various formats"""
    from datetime import datetime
    from django.utils import timezone

    if not timestamp_data:
        return None

    try:
        # Handle different timestamp formats
        if isinstance(timestamp_data, dict):
            # Format: {"number": 1234567890}
            if 'number' in timestamp_data:
                dt = datetime.fromtimestamp(timestamp_data['number'])
                return timezone.make_aware(dt)
            # Format: {"set": true, "infinite": false, "number": 1234567890}
            elif 'set' in timestamp_data and timestamp_data.get('set') and 'number' in timestamp_data:
                dt = datetime.fromtimestamp(timestamp_data['number'])
                return timezone.make_aware(dt)
        elif isinstance(timestamp_data, (int, float)):
            # Direct timestamp value
            dt = datetime.fromtimestamp(timestamp_data)
            return timezone.make_aware(dt)
        elif isinstance(timestamp_data, str):
            # String timestamp
            dt = datetime.fromtimestamp(float(timestamp_data))
            return timezone.make_aware(dt)
        elif isinstance(timestamp_data, list) and len(timestamp_data) > 0:
            # List format - take first element
            return _parse_slurm_timestamp(timestamp_data[0])
    except (ValueError, TypeError, KeyError) as e:
        logger.debug(f"Failed to parse timestamp {timestamp_data}: {e}")
        return None

    return None


def _create_external_job(job_data, cluster):
    """Create a new external job from SLURM data"""
    # Import here to avoid circular imports
    from ..models import Job, ClusterPartition, User, Role
    from django.db import transaction
    from django.utils import timezone
    from datetime import datetime
    from decimal import Decimal

    # Ensure slurm_jobid is an integer
    slurm_jobid_raw = job_data.get('slurm_jobid')
    if isinstance(slurm_jobid_raw, list):
        slurm_jobid = slurm_jobid_raw[0] if slurm_jobid_raw else None
        logger.warning(f"slurm_jobid was a list, using first element: {slurm_jobid}")
    else:
        slurm_jobid = slurm_jobid_raw

    user_name = job_data.get('user_name', 'unknown')
    partition_name = job_data.get('partition', 'default')
    job_name = job_data.get('name', '')

    # Determine job type based on job name pattern
    job_type = 'external'
    if job_name and '-install' in job_name:
        job_type = 'installation'
        logger.debug(f"Detected installation job from name pattern: {job_name}")

    # Normalize user_name if it's a list or other format
    if isinstance(user_name, list):
        user_name = user_name[0] if user_name else 'unknown'
        logger.debug(f"user_name was a list, using first element: {user_name}")

    # Ensure user_name is a valid string
    if not isinstance(user_name, str) or not user_name.strip():
        user_name = 'unknown'
        logger.warning(f"Invalid user_name, using 'unknown': {job_data.get('user_name')}")

    # Clean the username to ensure it's valid
    user_name = user_name.strip()
    if not user_name:
        user_name = 'unknown'

    # Get or create user
    try:
        user, created = User.objects.get_or_create(
            username=user_name,
            defaults={
                'email': f'{user_name}@external.local',
                'first_name': user_name,
                'last_name': 'External'
            }
        )

        if created:
            default_role = Role.objects.get(id=Role.NORMALUSER)
            user.roles.add(default_role)
            logger.debug(f"Created new user: {user_name}")
    except Exception as e:
        logger.error(f"Error creating/finding user '{user_name}': {e}")
        # Fallback to a default user
        try:
            user = User.objects.get(username='unknown')
        except User.DoesNotExist:
            # Create a fallback user
            user = User.objects.create(
                username='unknown',
                email='unknown@external.local',
                first_name='Unknown',
                last_name='External'
            )
            default_role = Role.objects.get(id=Role.NORMALUSER)
            user.roles.add(default_role)
            logger.info("Created fallback user 'unknown'")

    # Get partition
    try:
        partition = ClusterPartition.objects.get(
            cluster=cluster,
            name=partition_name
        )
    except ClusterPartition.DoesNotExist:
        # Create default partition if needed
        partition = ClusterPartition.objects.create(
            cluster=cluster,
            name=partition_name,
            machine_type='unknown',
            dynamic_node_count=0,
            static_node_count=0
        )

    # Parse timestamps
    start_time = None
    end_time = None

    if job_data.get('slurm_start_time'):
        start_time = _parse_slurm_timestamp(job_data['slurm_start_time'])

    if job_data.get('slurm_end_time'):
        end_time = _parse_slurm_timestamp(job_data['slurm_end_time'])

    # Calculate runtime
    runtime = None
    if start_time and end_time:
        runtime = (end_time - start_time).total_seconds()

    # Map SLURM status to OFE status
    slurm_status, additional_states = _normalize_slurm_status(job_data.get('slurm_status', ''))
    ofe_status = _map_slurm_status(slurm_status, job_data.get('exit_code'))

    # Extract exit code if available
    exit_code = job_data.get('exit_code')

    # Extract numeric values using helper function
    nodes_allocated = _extract_number(job_data.get('nodes_allocated'), 1)
    ntasks_per_node = _extract_number(job_data.get('ntasks_per_node'), 1)
    cpus_per_task = _extract_number(job_data.get('cpus_per_task'), 1)
    time_limit = _extract_number(job_data.get('time_limit'), 0)

    job_data_dict = {
        'name': job_data.get('name', f'External Job {slurm_jobid}'),
        'user': user,
        'cluster': cluster,
        'partition': partition,
        'application': None,  # External jobs don't have applications
        'number_of_nodes': nodes_allocated,
        'ranks_per_node': ntasks_per_node,
        'threads_per_rank': cpus_per_task,
        'wall_clock_time_limit': time_limit // 60,  # Convert to minutes
        'run_script': f'External job {slurm_jobid}',
        'status': ofe_status,
        'slurm_jobid': slurm_jobid,
        'slurm_status': slurm_status,
        'slurm_additional_states': additional_states if additional_states else [],
        'slurm_start_time': start_time,
        'slurm_end_time': end_time,
        'slurm_exit_code': exit_code,
        'runtime': runtime,
        'job_cost': Decimal('0.00'),  # External jobs don't have cost tracking
        'date_time_submission': start_time or timezone.now(),
        'job_type': job_type,
    }

    with transaction.atomic():
        job = Job.objects.create(**job_data_dict)
    logger.info(f'Created {job_type} job {job.id} for SLURM job {slurm_jobid}')


def _map_slurm_status(slurm_status, exit_code=None):
    """Map SLURM job status to OFE job status, considering exit code for completion"""
    status_mapping = {
        'PENDING': 'q',
        'CONFIGURING': 'q',
        'RUNNING': 'r',
        'COMPLETING': 'r',
        'SUSPENDED': 'q',
        'REQUEUED': 'q',
        'FAILED': 'e',
        'CANCELLED': 'e',
        'TIMEOUT': 'e',
        'PREEMPTED': 'e',
    }

    # Handle COMPLETED status based on exit code
    if slurm_status == 'COMPLETED':
        if exit_code is not None:
            # Determine success based on exit code
            job_success = _is_job_successful(slurm_status, exit_code)
            return 'c' if job_success else 'e'
        else:
            # If no exit code available, default to completed successfully
            return 'c'

    return status_mapping.get(slurm_status, 'n')


def cb_slurm_queue_status(message, source_id=None):
    """Handle SLURM queue status updates"""
    # Import here to avoid circular imports
    from ..models import SlurmQueueStatus, Cluster, ClusterPartition
    from django.db import transaction

    data = message.get("data", {})
    cluster_id = message.get("cluster_id")

    if not cluster_id:
        logger.warning("Received SLURM queue status without cluster ID")
        return True

    try:
        cluster = Cluster.objects.get(id=cluster_id, status='r')
    except Cluster.DoesNotExist:
        logger.warning(f"Cluster {cluster_id} not found or not ready")
        return True

    partition_name = data.get("partition", "default")
    queue_stats = data.get("queue_stats", {})
    node_stats = data.get("node_stats", {})

    # Debug logging
    logger.debug(f"SLURM queue status for cluster {cluster_id}, partition {partition_name}: queue_stats={queue_stats}, node_stats={node_stats}")
    logger.debug(f"Full queue_stats data: {json.dumps(queue_stats, indent=2)}")
    logger.debug(f"Full node_stats data: {json.dumps(node_stats, indent=2)}")

    try:
        partition = ClusterPartition.objects.get(
            cluster=cluster,
            name=partition_name
        )
    except ClusterPartition.DoesNotExist:
        logger.warning(f"Partition {partition_name} not found for cluster {cluster_id}")
        return True

    try:
        with transaction.atomic():
            queue_status = SlurmQueueStatus.objects.create(
                cluster=cluster,
                partition=partition,
                pending_jobs=queue_stats.get("pending", 0),
                running_jobs=queue_stats.get("running", 0),
                completed_jobs=queue_stats.get("completed", 0),
                available_nodes=node_stats.get("available", 0),
                total_nodes=node_stats.get("total", 0)
            )
        logger.debug(f"Updated queue status for cluster {cluster_id}, partition {partition_name}: pending={queue_status.pending_jobs}, running={queue_status.running_jobs}")
    except Exception as e:
        logger.error(f"Error updating queue status for cluster {cluster_id}: {e}")

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
        if utils.is_local_mode():
            logger.info("Starting C2 in local development mode - PubSub operations will be bypassed")
            return
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
    register_command("SLURM_JOB_UPDATE", cb_slurm_job_update)
    register_command("SLURM_QUEUE_STATUS", cb_slurm_queue_status)


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
            elif "FAIL" in text or "ERROR" in text or "CANCELLED" in text:
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
