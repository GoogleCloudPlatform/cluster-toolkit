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

""" models.py """

import itertools
import json
from django.db import models
from django.contrib.auth.models import AbstractUser
from django.core.exceptions import ValidationError
from django.core.validators import MinValueValidator
from django.core.validators import RegexValidator
from django.db.models.signals import post_save
from django.dispatch import receiver
from django.conf import settings
from rest_framework.authtoken.models import Token
from allauth.socialaccount.models import SocialAccount

CLOUD_RESOURCE_MGMT_STATUS = (
    ('i',  'Imported'),        # Use an existing resource created outside this system
    ('nm', 'New'),             # Just defined, managed
    ('cm', 'Creating'),        # In the process of creating, managed
    ('m',  'Managed/Running'), # Created, operational, managed
    ('dm', 'Destroying'),      # In the process of deleting, managed
    ('xm', 'Destroyed'),       # Deleted, managed
)

# Create your models here.

class Role(models.Model):
    """ Model representing different user roles """
    CLUSTERADMIN = 1
    NORMALUSER = 2
    VIEWER = 3
    ROLE_CHOICES = (
        (CLUSTERADMIN, 'cluster administrator'),
        (NORMALUSER, 'normal user'),
        (VIEWER, 'viewer'),
    )
    id = models.PositiveSmallIntegerField(
        choices = ROLE_CHOICES,
        primary_key = True,
    )

    def __str__(self):
        return self.get_id_display()


class User(AbstractUser):
    """ A custom User model extending the base Django one """

    roles = models.ManyToManyField(Role)
    ssh_key = models.TextField(
        max_length = 3072,
        help_text = 'If required, provide your public key to SSH into the cluster head node',
        blank = True,
        null = True,
    )
    # this field is set automatically from the post_save signal
    unix_id = models.PositiveIntegerField(
        validators = [MinValueValidator(1000)],
        help_text = "Unix ID for the user on the clusters and fileystems",
        blank = True,
        null = True,
    )

    def get_avatar_url(self):
        """ If using social login, return the Google profile picture if available """
        url = "/static/img/unknown_user.png"
        # SocialAccount table has 'extra_data' field containing the URL to extract
        if SocialAccount.objects.filter(user=self.id).exists():
            extra_data = SocialAccount.objects.get(user=self.id).extra_data
            json_data = json.dumps(extra_data)
            data = json.loads(json_data)
            url = data["picture"]
        return url

    def has_viewer_role(self):
        if self.roles.filter(id=3).exists():
            return True
        else:
            return False

    def has_normaluser_role(self):
        if self.roles.filter(id=2).exists():
            return True
        else:
            return False

    def has_admin_role(self):
        if self.roles.filter(id=1).exists():
            return True
        else:
            return False


@receiver(post_save, sender=settings.AUTH_USER_MODEL)
def user_post_save(sender, instance=None, created=False, **kwargs):
    """ Initialise certain information for new users """
    if created:
        # generate API token
        Token.objects.create(user=instance)
        # assign a UNIX ID to this user
        instance.unix_id = instance.id + 9999
        instance.save()
        # by default set new user to 'ordinary user'
        if instance.id > 1:
            instance.roles.set([Role.NORMALUSER])


def validate_domain_or_email(value):
    tmp = value
    if value.startswith('@'):
        tmp = 'dummy' + tmp
    import re
    regex = r'\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b'
    if not re.fullmatch(regex, tmp):
        raise ValidationError(
            "Input must be a valid email address for an individual user, or a domain name prepended with @ for a group of users.",
            params={"value": value}
        )

class AuthorisedUser(models.Model):
    """ Model to hold users allowed to access this system """
    pattern = models.CharField(
        max_length = 60,
        help_text = 'Enter a domain name starting with @ to authorise a group of users or an email address to authorise an individual user',
        validators=[validate_domain_or_email],
    )

    def __str__(self):
        return self.pattern


class Credential(models.Model):
    """ Model reprenseting a credential on a cloud platform """

    name = models.CharField(
        max_length = 30,
        help_text = 'Enter a name for this credential, e.g. My GCP credential',
    )
    owner = models.ForeignKey(
        User,
        help_text = 'Who owns this credential?',
        on_delete = models.RESTRICT,
    )
    detail = models.TextField(
        max_length = 4000,
        help_text = 'Obtain the credential detail and copy/paste it into this text field.',
    )

    def __str__(self):
        return self.name


class MachineType(models.Model):
    """ Model representing a virtual machine family on a cloud platform """

    name = models.CharField(
        max_length = 30,
        help_text = 'Enter a valid machine type',
        unique=True,
    )
    cpu_arch = models.CharField(
        max_length = 16,
        help_text = 'Processor architecture'
    )

    def __str__(self):
        return self.name


class InstanceType(models.Model):
    """ Model represent individual instance types/sizes """
    name = models.CharField(
                max_length=30,
                help_text = 'Instance Type/Size Name',
                unique=True
                )
    family = models.ForeignKey(
                MachineType,
                on_delete = models.RESTRICT,
                )
    num_vCPU = models.PositiveIntegerField(
                validators = [MinValueValidator(1)],
                help_text = 'Number of vCPU for this size'
    )

    @property
    def cpu_arch(self):
        return self.family.cpu_arch

    def __str__(self):
        return self.name


class CloudResource(models.Model):
    """ The base class of all cloud resource """
    cloud_credential = models.ForeignKey(
        Credential,
        help_text = 'Choose the credential to use with this cloud resource',
        on_delete = models.RESTRICT,
    )
    cloud_id = models.CharField(
        max_length = 4096,
        help_text = 'Cloud Resource id (GCP name, etc...)',
        blank = True,
        null = True,
    )
    cloud_state = models.CharField(
        max_length = 2,
        choices = CLOUD_RESOURCE_MGMT_STATUS,
        default = 'nm',
        help_text = "Current state of this cloud resource",
    )
    cloud_region = models.CharField(
        max_length = 30,
        help_text = 'The region of this cloud resource',
    )
    cloud_zone = models.CharField(
        max_length = 30,
        help_text = 'The zone of this cloud resource',
        blank = True,
        null = True,
    )

    @property
    def project_id(self):
        if self.cloud_credential:
            credInfo = json.loads(self.cloud_credential.detail)
            return credInfo.get("project_id", None)
        return None

    @property
    def is_managed(self):
        return 'm' in self.cloud_state


class VirtualNetwork(CloudResource):
    """ Model representing a virtual network (VPC) in the cloud """
    name = models.CharField(
        max_length = 64,
        help_text = 'Name for the virtual network',
    )

    def __str__(self):
        return self.name

    def in_use(self):
        for wb in Workbench.objects.all():
            if self == wb.subnet.vpc:
                #print(vpc)
                return True

        for cluster in Cluster.objects.all():
            if self == cluster.subnet.vpc:
                return True

        for fs in Filesystem.objects.all():
            if self == fs.subnet.vpc:#
                return True

        return False


class VirtualSubnet(CloudResource):
    """ Model representing a subnet in the cloud """
    name = models.CharField(
        max_length = 64,
        help_text = 'Name for the virtual subnet',
    )
    vpc = models.ForeignKey(
        VirtualNetwork,
        related_name = 'subnets',
        help_text = 'The VPC to which this subnet belongs',
        on_delete = models.CASCADE,
    )
    cidr = models.CharField(
        max_length=18,
        help_text = 'CIDR for this subnet',
    )

    def __str__(self):
        return f"{self.vpc.cloud_id} - {self.vpc.name} - {self.name}"


FILESYSTEM_TYPES = (
    (' ', "none"),
    ('n', "nfs"),
    ('l', "lustre"),
    ('d', "daos"),
    ('b', 'beegfs'),
)

class FilesystemImpl(models.IntegerChoices):
    BUILT_IN = 0, 'Cluster Built-in'
    GCPFILESTORE = 1, 'GCP Filestore'
    IMPORTED = 2, 'Imported Filesystem'


class Filesystem(CloudResource):
    """ Model representing a file system in the cloud """
    name = models.CharField(
        max_length = 40,
        help_text = 'Enter a name for the file system',
    )
    internal_name = models.CharField(
        max_length = 40,
        help_text = 'name generated by system (not to be set by user)',
        blank = True,
        null = True,
    )
    subnet = models.ForeignKey(
        VirtualSubnet,
        related_name = 'filesystems',
        help_text = 'Subnet within which the Filesystem resides (if any)',
        on_delete = models.RESTRICT,
        null = True,
        blank = True,
    )
    vpc = models.ForeignKey(
        VirtualNetwork,
        related_name = 'filesystems',
        help_text = 'Network within which the Filesystem resides',
        on_delete = models.SET_NULL,
        null = True,
    )
    impl_type = models.PositiveIntegerField(
        choices = FilesystemImpl.choices,
        blank = False,
    )
    fstype = models.CharField(
        max_length = 1,
        choices = FILESYSTEM_TYPES,
        help_text = 'Type of Filesystem (NFS, Lustre, etc)',
        blank = False,
        default = FILESYSTEM_TYPES[0][0]
    )

    @property
    def fstype_name(self):
        return dict(FILESYSTEM_TYPES).get(self.fstype)

    hostname_or_ip = models.CharField(
        max_length = 128,
        help_text = "Hostname or IP address of Filesystem server",
        null = True,
        blank = True,
    )

    def __str__(self):
        return self.name


class FilesystemExport(models.Model):
    """ Model representing a file system export """
    # mount -t <fstype> <server_name>:<export_name> /mnt

    filesystem = models.ForeignKey(
        Filesystem,
        related_name = 'exports',
        on_delete = models.CASCADE
    )
    @property
    def fstype(self):
        return self.filesystem.fstype

    @property
    def fstype_name(self):
        return self.filesystem.fstype_name

    @property
    def server_name(self):
        return self.filesystem.hostname_or_ip

    export_name = models.CharField(
        max_length=256,
        help_text = "An export from NFS, or name of FS for Lustre, etc.",
    )

    @property
    def source_string(self):
        if self.server_name:
            return f"{self.server_name}:{self.export_name}"
        else:
            return f"{self.export_name}"

    def __str__(self):
        return f"{self.export_name} on {self.filesystem}"


class MountPoint(models.Model):
    """ Model representing a mount point """
    export = models.ForeignKey(
        FilesystemExport,
        related_name = '+',
        on_delete = models.CASCADE,
    )

    cluster = models.ForeignKey(
        "Cluster",
        related_name = "mount_points",
        on_delete = models.CASCADE,
    )

    @property
    def fstype(self):
        return self.export.fstype

    @property
    def fstype_name(self):
        return self.export.fstype_name

    @property
    def mount_source(self):
        return self.export.source_string

    mount_order = models.PositiveIntegerField(
        help_text = 'Mounts are mounted in numerically increasing order',
        default = 0,
    )

    mount_options = models.CharField(
        max_length = 128,
        help_text = "Mount options (passed to mount -o)",
        blank = True,
    )

    mount_path = models.CharField(
        max_length = 4096,
        help_text = "Path on which to mount this filesystem",
    )

    def __str__(self):
        return f"{self.mount_path} on {self.cluster}"


class Cluster(CloudResource):
    """ Model representing a cluster """

    name = models.CharField(
        max_length = 40,
        help_text = 'Enter a name for the cluster',
    )
    owner = models.ForeignKey(
        User,
        related_name = 'owner',
        help_text = 'Who owns this cluster?',
        on_delete = models.RESTRICT,
    )
    subnet = models.ForeignKey(
        VirtualSubnet,
        related_name = 'clusters',
        help_text = 'Subnet within which the cluster resides',
        on_delete = models.RESTRICT,
    )
    authorised_users = models.ManyToManyField(
        User,
	related_name = 'authorised_users',
        help_text = 'Select other users authorised to use this cluster',
    )
    CLUSTER_STATUS = (
        ('n', 'Cluster is being newly configured by user'),
        ('c', 'Cluster is being created'),
        ('i', 'Cluster is being initialised'),
        ('r', 'Cluster is ready for jobs'),
        ('s', 'Cluster is stopped (can be restarted)'),
        ('t', 'Cluster is terminating'),
        ('e', 'Cluster deployment has failed'),
        ('d', 'Cluster has been deleted'),
    )
    status = models.CharField(
        max_length = 1,
        choices = CLUSTER_STATUS,
        default = 'n',
        help_text = 'Status of this cluster',
    )
    spackdir = models.CharField(
        max_length = 4096,
        verbose_name = "Spack directory",
        default = "/opt/cluster/spack",
        help_text = 'Specify where Spack install applications on the cluster',
    )
    shared_fs = models.ForeignKey(
        Filesystem,
        on_delete = models.RESTRICT,
        related_name = '+',
    )
    spack_install = models.ForeignKey(
        'ApplicationInstallationLocation',
        on_delete = models.SET_NULL,
        related_name = 'clusters_using',
        null = True,
        blank = True,
    )
    controller_node = models.OneToOneField(
        'ComputeInstance',
        on_delete = models.SET_NULL,
        null = True,
        blank = True,
    )
    num_login_nodes = models.PositiveIntegerField(
        validators = [MinValueValidator(0)],
        help_text = 'The number of login nodes to create',
        default = 1,
    )
    def get_access_key(self):
        return Token.objects.get(user=self.owner)

    def __str__(self):
        """String for representing the Model object."""
        return f"Cluster '{self.name}'"


class ComputeInstance(CloudResource):
    cluster_login = models.ForeignKey(
        Cluster,
        related_name = "login_nodes",
        unique = False,
        null = True,
        blank = True,
        on_delete = models.CASCADE,
    )
    internal_ip = models.GenericIPAddressField(
        protocol = 'IPv4',
        blank = True,
        null = True,
    )
    public_ip = models.GenericIPAddressField(
        protocol = 'IPv4',
        blank = True,
        null = True,
    )
    instance_type = models.ForeignKey(
        InstanceType,
        related_name = '+',
        # Probably shouldn't ever be not set, but we don't *require* it for anything yet.
        on_delete = models.SET_NULL,
        null = True,
        blank = True,
    )
    service_account = models.EmailField(
        max_length = 512,
        null = True,
        blank = True,
        default = "",
    )


class ClusterPartition(models.Model):
# TODO - SlurmGCP allows subnet & zone specification on Partition
    name = models.CharField(
        max_length = 80,
        help_text = 'Partition name'
    )
    cluster = models.ForeignKey(
        Cluster,
        related_name = "partitions",
        on_delete = models.CASCADE,
    )
    machine_type = models.ForeignKey(
        InstanceType,
        related_name = '+',
        on_delete = models.RESTRICT,
    )
    image = models.CharField(
        max_length = 4096,
        help_text = 'OS Image path',
        blank = True,
    )
    max_node_count = models.PositiveIntegerField(
        validators = [MinValueValidator(1)],
        help_text = 'The maximum number of nodes in the partition',
        default = 2,
    )
    enable_placement = models.BooleanField(
        default = True,
        help_text = 'Enable Placement Groups (currently only valid for C2 and C2D instances)'
    )
    enable_hyperthreads = models.BooleanField(
        default = False,
        help_text = 'Enable Hyprethreads (SMT)'
    )
    enable_node_reuse = models.BooleanField(
        default = True,
        help_text = 'Enable nodes to be re-used for multiple jobs. (Disabled when Placement Groups are used.)'
    )

    @property
    def vCPU_per_node(self):
        return self.machine_type.num_vCPU // (1 if self.enable_hyperthreads else 2)


    def __str__(self):
        return self.name


class ApplicationInstallationLocation(models.Model):
    fs_export = models.ForeignKey(
        FilesystemExport,
        on_delete = models.CASCADE,
        help_text = 'Filestore on which the application resides'
    )
    path = models.CharField(
        max_length = 2048,
        help_text = 'Directory in the filestore where application resides'
    )
    @property
    def filesystem(self):
        return self.fs_export.filesystem


class Application(models.Model):
    """ Model representing a particular binary installation of an application. """

    name = models.CharField(
        max_length = 30,
        help_text = 'Enter an application name',
    )
    description = models.TextField(
        max_length = 4000,
        help_text = '(Optional) description of this application',
        blank = True,
        null = True,
    )
    version = models.CharField(
        max_length = 30,
        help_text = '(Optional) which version of this application',
        blank = True,
        null = True,
    )
    # We store both the cluster and the installation location
    # This allows us to track which cluster was used to perform the installation
    cluster = models.ForeignKey(
        Cluster,
        help_text = 'Which cluster was used to install the application',
        on_delete = models.CASCADE,
    )
    install_loc = models.ForeignKey(
        ApplicationInstallationLocation,
        help_text = 'Location of the application installation',
        on_delete = models.CASCADE,
        blank = True,
        null = True,
    )
    install_partition = models.ForeignKey(
        ClusterPartition,
        help_text = 'Cluster partition on which the installation job will be run',
        on_delete = models.RESTRICT,
        blank = True,
        null = True,
    )
    installed_architecture = models.CharField(
        max_length = 128,
        help_text = 'CPU architecture of installed package',
        blank = True,
        null = True,
    )
    load_command = models.CharField(
        max_length = 200,
        help_text = \
            "Commands to load the application package, e.g. 'spack load xxx' or 'module load yyy'",
        blank = True,
        null = True,
    )
    compiler = models.CharField(
        max_length = 40,
        help_text = "Which compiler was used to build this application",
        blank = True,
        null = True,
    )
    mpi = models.CharField(
        max_length = 40,
        help_text = "Which MPI library was this application built against",
        blank = True,
        null = True,
    )
    APPLICATION_INSTALLATION_STATUS = (
        ('n', 'Application is being newly configured'),
        ('p', 'Application installation is being prepared'),
        ('q', 'Application installation is in job queue'),
        ('i', 'Application is being installed'),
        ('r', 'Application successfully installed and ready to run'),
        ('e', 'Application installation completed in error'),
        ('x', 'Hosting cluster has been destroyed'),
    )
    status = models.CharField(
        max_length = 1,
        choices = APPLICATION_INSTALLATION_STATUS,
        default = 'n',
        help_text = 'Status of this application installation',
    )

    def __str__(self):
        """String for representing the Model object."""
        return f'{self.name} - {self.get_status_display()}'


class CustomInstallationApplication(Application):
    install_script = models.CharField(
        max_length = 8192,
        help_text= \
            'The URL to a an installation script, or the raw script',
    )

    module_name = models.CharField(
        max_length = 128,
        help_text = 'name of module file to install, and load',
        blank = True,
        null = True,
    )

    module_script = models.CharField(
        max_length = 8192,
        help_text = 'environment modules file to install to load application',
        blank = True,
        null = True,
    )


class SpackApplication(Application):
    spack_name = models.CharField(
        max_length = 30,
        help_text = 'Name of the application in Spack',
        blank = True,
        null = True,
    )
    spack_spec = models.CharField(
        max_length = 200,
        help_text = 'Spack spec that refers to this particular build configuration',
        blank = True,
        null = True,
    )
    spack_hash = models.CharField(
        max_length=32,
        help_text = 'Hash of the Spack installation of the application package',
        blank = True,
        null = True,
    )




class Benchmark(models.Model):
    """ Model representing a benchmark """

    name = models.CharField(
        max_length = 30,
        help_text = 'Enter a name of this benchmark',
    )
    description = models.TextField(
        max_length = 4000,
        help_text = 'Enter a description of this benchmark',
    )

    def __str__(self):
        """String for representing the Benchmark object."""
        return self.name

class Job(models.Model):
    """ Model representing a single run of an application """

    application = models.ForeignKey(
        Application,
        help_text = 'Which application installation to use?',
        on_delete = models.RESTRICT,
    )
    cluster = models.ForeignKey(
        Cluster,
        help_text = 'Which cluster was used for this job',
        on_delete = models.SET_NULL,
        null = True,
    )
    name = models.CharField(
        max_length = 40,
        help_text = 'Enter a job name',
    )
    date_time_submission = models.DateTimeField(
        blank = True,
        null = True,
        auto_now_add=True,
    )
    user = models.ForeignKey(
        User,
        help_text = 'Who owns this job?',
        on_delete = models.CASCADE,
    )
    partition = models.ForeignKey(
        ClusterPartition,
        help_text = 'Cluster partition on which the job will be run',
        on_delete = models.CASCADE,
    )
    number_of_nodes = models.PositiveIntegerField(
        validators = [MinValueValidator(1)],
        help_text = 'The number of nodes to use',
    )
    ranks_per_node = models.PositiveIntegerField(
        validators = [MinValueValidator(1)],
        help_text = 'The number of MPI ranks per node',
    )
    threads_per_rank = models.PositiveIntegerField(
        validators = [MinValueValidator(1)],
        default = 1,
        help_text = 'The number of threads per MPI rank (for hybrid jobs)',
    )
    wall_clock_time_limit = models.PositiveIntegerField(
        validators = [MinValueValidator(0)],
        default = 0,
        help_text = 'The wall clock time limit of this job (in minutes)',
        blank = True,
        null = True,
    )
    run_script = models.CharField(
        max_length = 8192,
        help_text= \
            'The URL to the job script (a shell script or a tarball containing run.sh). Or the raw script',
    )
    # adpated from Django's URL validation regex
    import re
    ul = '\u00a1-\uffff'
    hostname_re = r'[a-z' + ul + r'0-9](?:[a-z' + ul + r'0-9-]{0,61}[a-z' + ul + r'0-9])?'
    domain_re = r'(?:\.(?!-)[a-z' + ul + r'0-9-]{1,63}(?<!-))*'
    tld_re = (
        r'\.'                                # dot
        r'(?!-)'                             # can't start with a dash
        r'(?:[a-z' + ul + '-]{2,63}'         # domain label
        r'|xn--[a-z0-9]{1,59})'              # or punycode label
        r'(?<!-)'                            # can't end with a dash
        r'\.?'                               # may have a trailing dot
    )
    host_re = '(' + hostname_re + domain_re + tld_re + '|' + hostname_re + '|localhost)'
    ipv4_re = r'(?:25[0-5]|2[0-4]\d|[0-1]?\d?\d)(?:\.(?:25[0-5]|2[0-4]\d|[0-1]?\d?\d)){3}'
    cloud_storage_url_regex = re.compile(
        r'^(?:http|https|gs|s3)://' # schemes
        r'(?:' + ipv4_re + '|' + host_re + ')'
        r'(?::\d{2,5})?'  # port
        r'(?:[/?#][^\s]*)?'  # resource path
        r'\Z', re.IGNORECASE)

    cloud_storage_url_validator = RegexValidator(
        cloud_storage_url_regex,
        message = 'Error validating cloud storage URL',
    )
# Note: Cannot use URLField here.
# The Django URLValidator has issues:
# 1) FieldValidator here doesn't get matched to Form's Field Validator
# 2) URLValidator doesn't support hostnames without a TLD, so things like:
# gs://mcbench/foo/bar    Are considered invalid.
# At some point, a new validator should be written, but not today.
    input_data = models.CharField(
        max_length = 200,
        help_text = '(Optional) the URL to download input dataset',
        blank = True,
        validators = [cloud_storage_url_validator],
    )
    result_data = models.CharField(
        max_length = 200,
        help_text = '(Optional) the URL to upload result dataset',
        blank = True,
        validators = [cloud_storage_url_validator],
    )
    JOB_STATUS = (
        ('n', 'A new job has been created and is being configured'),
        ('p', 'Job is being prepared'),
        ('q', 'Job is in a queue'),
        ('d', 'Job input dataset is being downloaded from long-term storage'),
        ('r', 'Job is running on the cluster'),
        ('u', 'Job result dataset is being uploaded to long-term storage'),
        ('c', 'Job has completed successfully'),
        ('e', 'Job has completed in error'),
    )
    slurm_jobid = models.PositiveIntegerField(
        blank = True,
        null = True,
        help_text = 'SLURM Job ID',
    )
    status = models.CharField(
        max_length = 1,
        choices = JOB_STATUS,
        default = 'n',
        help_text = 'Status of this job',
    )
    runtime = models.FloatField(
        help_text = 'Job run time (in seconds)', # as reported by scheduler
        blank = True,
        null = True,
    )
    cost = models.DecimalField(
        max_digits=8,
        decimal_places=3,
        help_text = 'Job cost hour rate',
        blank = True,
        null = True,
    )
    result_unit = models.CharField(
        max_length = 20,
        help_text = 'The unit of a key performance indicator',
        blank = True,
        null = True,
    )
    result_value = models.FloatField(
        help_text = 'The value of a key performance indicator',
        blank = True,
        null = True,
    )
    CLEANUP_OPTIONS = (
        ('a', 'Always remove job files'),
        ('e', 'Only remove job files on error'),
        ('s', 'Only remove job files on success'),
        ('n', 'Never remove job files'),
    )
    cleanup_choice = models.CharField(
        max_length = 1,
        choices = CLEANUP_OPTIONS,
        default = 'n',
        help_text = 'When to remove job files',
    )
    benchmark = models.ForeignKey(
        Benchmark,
	help_text = '(Optional) Identify the benchmark this job belongs to',
        on_delete = models.RESTRICT,
        blank = True,
        null = True,
    )

    def __str__(self):
        """String for representing the Model object."""
        return f"#{self.id} - '{self.name}' on {self.application.cluster}"


class Task(models.Model):
    owner = models.ForeignKey(
        User,
        help_text = 'Who is running the task',
        on_delete = models.RESTRICT,
    )
    title = models.CharField(
        max_length = 128,
    )
    data = models.JSONField(
        blank = True,
        null = False,
        default = dict,
    )


class GCPFilestoreFilesystem(Filesystem):
    FILESTORE_TIER = (
        ('u', 'TIER_UNSPECIFIED'),
        ('s', 'STANDARD'),
        ('p', 'PREMIUM'),
        ('bh', 'BASIC_HDD'),
        ('bs', 'BASIC_SSD'),
        ('hs', 'HIGH_SCALE_SSD'),
    )
    capacity = models.PositiveIntegerField(
        validators = [MinValueValidator(1024)],
        help_text = 'Capacity (in GB) of the filesystem (min of 2660)',
        default = 1024
    )

    performance_tier = models.CharField(
        max_length = 2,
        choices = FILESTORE_TIER,
        help_text = 'Filestore Performance Tier'
    )

    def save(self, *args, **kwargs):
        self.fstype = 'n'
        self.impl_type = FilesystemImpl.GCPFILESTORE
        super().save(*args, **kwargs)

    def __str__(self):
        return f"GCP Filestore {self.name}"


FILESYSTEM_IMPL_INFO = {
    FilesystemImpl.BUILT_IN: {'name': 'Cluster Built-in', 'class': None},
    FilesystemImpl.GCPFILESTORE: {'name': 'GCP Filestore', 'class': GCPFilestoreFilesystem, 'url-key': 'filestore', 'terraform_dir': 'gcp_filestore'},
    FilesystemImpl.IMPORTED: {'name': 'Imported Filesystem', 'class': Filesystem, 'url-key': 'import-fs'},
}

class WorkbenchPreset(models.Model):
    """ Model representing a vertex AI workbench """

    name = models.CharField(
        max_length = 40,
        primary_key = True,
        help_text = 'Enter a name for the Workbench Preset',
    )

    machine_type = models.CharField(
        max_length = 40,
        default = 'n1-standard-1',
        help_text = "The machine type for this workbench size",
    )

    category = models.CharField(
        max_length = 40,
        help_text = 'Enter the heading this preset should appear under',
    )
    # WORKBENCH_BOOTDISKTYPE = (
    #     ('PD_STANDARD', 'Standard Persistent Disk'),
    #     ('PD_BALANCED', 'Balanced Persistent Disk'),
    #     ('PD_SSD', 'SSD Persistent Disk'),
    # )
    # boot_disk_type = models.CharField(
    #     max_length = 11,
    #     choices = WORKBENCH_BOOTDISKTYPE,
    #     default = 'PD_STANDARD',
    #     help_text = 'Type of storage to be required for notebook boot disk',
    # )
    # boot_disk_capacity = models.PositiveIntegerField(
    #     validators = [MinValueValidator(100)],
    #     help_text = 'Capacity (in GB) of the filesystem (min of 1024)',
    #     default = 100
    # )


class Workbench(CloudResource):
    """ Model representing a vertex AI workbench """

    name = models.CharField(
        max_length = 40,
        help_text = 'Enter a name for the Workbench',
    )
    internal_name = models.CharField(
        max_length = 40,
        help_text = 'Workbench name generated by system (not to be set by user)',
        blank = True,
        null = True,
    )
    owner = models.ForeignKey(
        User,
        related_name = 'workbench_owner',
        help_text = 'Who owns this Workbench?',
        on_delete = models.RESTRICT,
    )
    subnet = models.ForeignKey(
        VirtualSubnet,
        related_name = 'workbench_subnet',
        help_text = 'Subnet within which the workbench resides',
        on_delete = models.RESTRICT,
    )
    WORKBENCH_STATUS = (
        ('n', 'Workbench is being newly configured by user'),
        ('c', 'Workbench is being created'),
        ('i', 'Workbench is being initialised'),
        ('r', 'Workbench is ready'),
        ('s', 'Workbench is stopped (can be restarted)'),
        ('t', 'Workbench is terminating'),
        ('e', 'Workbench deployment has failed'),
        ('d', 'Workbench has been destroyed'),
    )
    status = models.CharField(
        max_length = 1,
        choices = WORKBENCH_STATUS,
        default = 'n',
        help_text = 'Status of this cluster',
    )
    machine_type = models.CharField(
        max_length = 40,
        default = 'n1-standard-1',
        help_text = "The machine type for this workbench",
    )
    WORKBENCH_BOOTDISKTYPE = (
        ('PD_STANDARD', 'Standard Persistent Disk'),
        ('PD_BALANCED', 'Balanced Persistent Disk'),
        ('PD_SSD', 'SSD Persistent Disk'),
    )
    boot_disk_type = models.CharField(
        max_length = 11,
        choices = WORKBENCH_BOOTDISKTYPE,
        default = 'PD_STANDARD',
        help_text = 'Type of storage to be required for notebook boot disk',
    )
    boot_disk_capacity = models.PositiveIntegerField(
        validators = [MinValueValidator(100)],
        help_text = 'Capacity (in GB) of the filesystem (min of 1024)',
        default = 100
    )
    proxy_uri = models.CharField(
        max_length = 150,
        blank = True,
        null = True
    )
    trusted_users = models.ManyToManyField(
        User,
        help_text = 'Select other users authorised to use this cluster',
    )
    WORKBENCH_IMAGEFAMILIES = (
        ('common-cpu-notebooks', 'Base Python 3 (with Intel MKL)'),
        ('tf-latest-cpu', 'TensorFlow Enterprise (Intel® MKL-DNN/MKL)'),
        ('pytorch-latest-cpu', 'PyTorch'),
        ('r-latest-cpu-experimental', 'R (Experimental)')
    )
    image_family = models.CharField(
        max_length = 32,
        choices = WORKBENCH_IMAGEFAMILIES,
        default = 'base-cpu',
        help_text = 'Select the image family that you wish to use',
    )

    @property
    def get_access_key(self):
        return Token.objects.get(user=self.owner)

    def __str__(self):
        """String for representing the Model object."""
        return f"Workbench '{self.name}'"

    def list_trusted_users(self):
        return list(self.trusted_users.all())
