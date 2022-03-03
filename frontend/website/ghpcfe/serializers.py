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

""" serializers.py """

from rest_framework import serializers
from .models import Application, Cluster, Credential, Job, User, Task, \
                    VirtualNetwork, VirtualSubnet, MountPoint


class CredentialSerializer(serializers.ModelSerializer):
    """ Custom ModelSerializer for Credential model """

    class Meta:
        model = Credential
        fields = ('id', 'name', 'owner', 'detail')
        extra_kwargs = {'detail': {'write_only': True}}


class MountPointSerializer(serializers.ModelSerializer):
    class Meta:
        model = MountPoint
        fields = ('mount_path', 'fstype', 'mount_source', 'mount_options')


class ClusterSerializer(serializers.ModelSerializer):
    """ Custom ModelSerializer for Cluster model """
    cloud_vpc = serializers.CharField(source='subnet.vpc.cloud_id', read_only=True)
    cloud_subnet = serializers.CharField(source='subnet.cloud_id', read_only=True)
    mount_points = serializers.SerializerMethodField()
    def get_mount_points(self, instance):
        mps = instance.mount_points.all().order_by('mount_order')
        return MountPointSerializer(mps, many=True, read_only=True).data
    class Meta:
        model = Cluster
        fields = ('id', 'name', 'internal_name', 'cloud_region', 'cloud_zone',
                  'head_node_ip', 'head_node_internal_ip', 'status', 'cloud_credential', 'advanced_networking',
                  'ansible_branch', 'cloud_vpc', 'cloud_subnet', 'spackdir',
                  'mount_points')


class ApplicationSerializer(serializers.ModelSerializer):
    """ Custom ModelSerializer for Application model """

    class Meta:
        model = Application
        fields = ('version', 'spack_name', 'spack_spec',
                  'spack_hash', 'load_command', 'compiler', 'mpi', 'status')


class JobSerializer(serializers.ModelSerializer):
    """ Custom ModelSerializer for Job model """
    user = serializers.CharField(source='user.username', read_only=True)
    instance_type = serializers.CharField(source='instance_type.name', read_only=True)
    class Meta:
        model = Job
        fields = ('application', 'user', 'name', 'date_time_submission',
                  'instance_type', 'number_of_nodes', 'ranks_per_node',
                  'threads_per_rank', 'wall_clock_time_limit', 'run_script',
                  'image_id', 'input_data', 'result_data', 'cleanup_choice',
                  'status', 'runtime', 'cost', 'result_unit', 'result_value')


class UserSerializer(serializers.ModelSerializer):
    """ Custom ModelSerializer for User model """

    class Meta:
        model = User
        fields = ('username', 'first_name', 'last_name', 'ssh_key', 'unix_id')


class TaskSerializer(serializers.ModelSerializer):
    """ Custom ModelSerializer for Task model """
    data = serializers.JSONField()
    class Meta:
        model = Task
        fields = ('owner', 'title', 'data')


class VirtualNetworkSerializer(serializers.ModelSerializer):
    """ Custom ModelSerializer for VirtualNetwork model """

    class Meta:
        model = VirtualNetwork
        fields = ('name', 'cloud_id', 'cloud_region')


class VirtualSubnetSerializer(serializers.ModelSerializer):
    """ Custom ModelSerializer for VirtualSubnet model """
    vpc = serializers.CharField(source='vpc.cloud_id', read_only=True)

    class Meta:
        model = VirtualSubnet
        fields = ('name', 'vpc', 'cidr', 'cloud_id', 'cloud_region')
