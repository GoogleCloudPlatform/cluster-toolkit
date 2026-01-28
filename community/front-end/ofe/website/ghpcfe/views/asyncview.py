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
""" asyncviews.py """
import asyncio
import functools
import logging

from asgiref.sync import sync_to_async
from django.core import exceptions
from django.utils.decorators import classonlymethod
from django.views import generic
from rest_framework import viewsets
from rest_framework.authentication import SessionAuthentication
from rest_framework.authentication import TokenAuthentication
from rest_framework.authtoken.models import Token
from rest_framework.permissions import IsAuthenticated

from ..models import Cluster, Role, Task
from ..serializers import TaskSerializer

logger = logging.getLogger(__name__)


class RunningTasksViewSet(viewsets.ModelViewSet):
    permission_classes = (IsAuthenticated,)
    queryset = Task.objects.all()
    serializer_class = TaskSerializer
    authentication_classes = [SessionAuthentication, TokenAuthentication]


def _consume_task(record, task):
    logger.info("Task %s complete.", task.get_name(), exc_info=task.exception())
    if record:
        logger.info(
            "    destroying task record %d-%s.  Data: %s",
            record.id,
            record.title,
            record.data
        )
        asyncio.create_task(sync_to_async(record.delete)())


class BackendAsyncView(generic.View):
    """Template class for backend async operations"""

    @classonlymethod
    def as_view(cls, **initkwargs):
        view = super().as_view(**initkwargs)
        view._is_coroutine = asyncio.coroutines._is_coroutine # pylint: disable=protected-access
        return view

    @sync_to_async
    def test_user_access_to_cluster(self, user, cluster_id):
        cluster = Cluster.objects.get(pk=cluster_id)
        if user not in cluster.authorised_users.all():
            raise exceptions.PermissionDenied

    @sync_to_async
    def test_user_is_cluster_admin(self, user):
        if Role.CLUSTERADMIN not in [x.id for x in user.roles.all()]:
            raise exceptions.PermissionDenied

    @sync_to_async
    def make_task_record(self, user, title):
        task_data = self.get_task_record_data(self.request)
        t = Task.objects.create(owner=user, title=title, data=task_data)
        t.save()
        return t

    @sync_to_async
    def get_user_token(self, user):
        token = Token.objects.get(user=user)
        return token.key

    @sync_to_async
    def set_cluster_status_async(self, cluster_id, status):
        self.set_cluster_status(cluster_id, status)

    def set_cluster_status(self, cluster_id, status):
        c = Cluster.objects.get(pk=cluster_id)
        c.status = status
        c.save()

    def get_task_record_data(self, request):
        """Called from a synchronous context"""
        return {}

    async def _cmd(self, *args, **kwargs):
        await sync_to_async(self.cmd, thread_sensitive=False)(*args, **kwargs)

    async def create_task(self, title, *args, **kwargs):
        logger.info("Creating task %s", title)
        token = await self.get_user_token(self.request.user)
        record = await self.make_task_record(self.request.user, title=title)
        task = asyncio.create_task(self._cmd(record.pk, token, *args, **kwargs))
        task.add_done_callback(functools.partial(_consume_task, record))
        return record
