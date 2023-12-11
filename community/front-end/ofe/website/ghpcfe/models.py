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

import base64
import json
import logging
import re
import ipaddress
import uuid
from decimal import Decimal

import dill
from allauth.socialaccount.models import SocialAccount
from django.conf import settings
from django.contrib.auth.models import AbstractUser
from django.core.exceptions import ValidationError
from django.core.validators import MinValueValidator, RegexValidator, MaxLengthValidator
from django.db import models
from django.db.models.signals import post_save
from django.dispatch import receiver
from rest_framework.authtoken.models import Token

logger = logging.getLogger(__name__)

CLOUD_RESOURCE_MGMT_STATUS = (
    ("i", "Imported"),  # Use an existing resource created outside this system
    ("nm", "New"),  # Just defined, managed
    ("cm", "Creating"),  # In the process of creating, managed
    ("m", "Managed/Running"),  # Created, operational, managed
    ("re", "Reconfiguring"),  # Created, operational, managed
    ("dm", "Destroying"),  # In the process of deleting, managed
    ("xm", "Destroyed"),  # Deleted, managed
    ("um", "Unknown"),  # Unknown, following error
)


# Name style matches the other options that Django provides
def RESTRICT_IF_CLOUD_ACTIVE(
    collector, field, sub_objs, using
):  # pylint: disable=invalid-name
    restrict_objs = []
    set_null_objs = []
    for obj in sub_objs:
        if obj.cloud_state == "xm":
            set_null_objs.append(obj)
        else:
            restrict_objs.append(obj)
    models.RESTRICT(collector, field, restrict_objs, using)
    models.SET_NULL(collector, field, set_null_objs, using)


# What else would I call it?
def RFC1035Validator(maxLength, message):  # pylint: disable=invalid-name
    if not maxLength:
        regex = re.compile("^[a-z][-a-z0-9]*[a-z0-9])$")
    elif maxLength < 2:
        raise ValueError("Max Length must be >= 2")
    # regex = re.compile(f'[a-z]([-a-z0-9]{{0,{maxLength-2}}}[a-z0-9])')
    else:
        regex = f"^[a-z]([-a-z0-9]{{0,{maxLength-2}}}[a-z0-9])$"
    return RegexValidator(regex, message=message)


def CIDRValidator(value):
    try:
        net = ipaddress.IPv4Network(value)

    # Note: ipaddress throws exception types beyond those documented
    except Exception as err:  # pylint: disable=broad-except
        raise ValidationError(
            "%(value)s is not a valid CIDR. Please provide a valid CIDR.",
            params={"value": value},
        ) from err
    if not net.is_private:
        raise ValidationError(
            "Only private IP addresses can be used in a VPC network."
        )

    return value


class Role(models.Model):
    """Model representing different user roles"""

    CLUSTERADMIN = 1
    NORMALUSER = 2
    VIEWER = 3
    ROLE_CHOICES = (
        (CLUSTERADMIN, "cluster administrator"),
        (NORMALUSER, "normal user"),
        (VIEWER, "viewer"),
    )
    id = models.PositiveSmallIntegerField(
        choices=ROLE_CHOICES,
        primary_key=True,
    )

    def __str__(self):
        return self.get_id_display()


class User(AbstractUser):
    """A custom User model extending the base Django one"""

    roles = models.ManyToManyField(Role)
    QUOTA_TYPE = (
        ("u", "Unlimited compute spend"),
        ("l", "Limited compute spend"),
        ("d", "Compute disabled"),
    )
    quota_type = models.CharField(
        max_length=1,
        choices=QUOTA_TYPE,
        default="d",
        help_text="User Compute Quota Type",
    )
    quota_amount = models.DecimalField(
        max_digits=8,
        decimal_places=2,
        help_text="Maximum allowed spend ($)",
        default=0,
    )

    def total_spend(self, date_range=None, cluster_id=None):
        filters = {"user": self.id}
        if date_range:
            filters["date_time_submission__range"] = date_range
        if cluster_id:
            filters["cluster"] = cluster_id

        jobs = Job.objects.filter(**filters)

        total_spend = Decimal(0)
        for job in jobs:
            total_spend += job.job_cost

        return total_spend

    def total_jobs(self, date_range=None, cluster_id=None):
        filters = {"user": self.id}
        if date_range:
            filters["date_time_submission__range"] = date_range
        if cluster_id:
            filters["cluster"] = cluster_id

        jobs = Job.objects.filter(**filters)

        return len(jobs)

    def quota_remaining(self):
        return self.quota_amount - self.total_spend()

    def check_sufficient_quota_for_job(self, job_cost):
        # Quota checks
        if self.quota_type == "u":
            return True
        if self.quota_type == "d":
            return False

        if self.quota_type == "l":
            current_used = self.total_spend()
            if (current_used + job_cost) < self.quota_amount:
                return True

        return False

    def get_avatar_url(self):
        """If using social login, return the Google profile picture if
        available"""

        url = "/static/img/unknown_user.png"
        # SocialAccount table has 'extra_data' field containing the URL to
        # extract
        if SocialAccount.objects.filter(user=self.id).exists():
            extra_data = SocialAccount.objects.get(user=self.id).extra_data
            json_data = json.dumps(extra_data)
            data = json.loads(json_data)
            url = data["picture"]
        return url

    def has_viewer_role(self):
        return bool(self.roles.filter(id=3).exists())

    def has_normaluser_role(self):
        return bool(self.roles.filter(id=2).exists())

    def has_admin_role(self):
        return bool(self.roles.filter(id=1).exists())


@receiver(post_save, sender=settings.AUTH_USER_MODEL)
def user_post_save(  # pylint: disable=invalid-name,unused-argument
    sender,
    instance=None,
    created=False,
    **kwargs,
):
    """Initialise certain information for new users"""
    if created:
        # generate API token
        Token.objects.create(user=instance)
        # by default set new user to 'ordinary user'
        if instance.id > 1:
            instance.roles.set([Role.NORMALUSER])


def validate_domain_or_email(value):  # pylint: disable=invalid-name
    tmp = value
    if value.startswith("@"):
        tmp = "dummy" + tmp

    regex = r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b"
    if not re.fullmatch(regex, tmp):
        raise ValidationError(
            "Input must be a valid email address for an individual user, or a "
            "domain name prepended with @ for a group of users.",
            params={"value": value},
        )


class AuthorisedUser(models.Model):
    """Model to hold users allowed to access this system"""

    pattern = models.CharField(
        max_length=60,
        help_text=(
            "Enter a domain name starting with @ to authorise a group "
            "of users or an email address to authorise an individual user"
        ),
        validators=[validate_domain_or_email],
    )

    def __str__(self):
        return self.pattern


class Credential(models.Model):
    """Model representing a credential on a cloud platform"""

    name = models.CharField(
        max_length=30,
        help_text="Enter a name for this credential, e.g. My GCP credential",
    )
    owner = models.ForeignKey(
        User,
        help_text="Who owns this credential?",
        on_delete=models.RESTRICT,
    )
    detail = models.TextField(
        max_length=4000,
        help_text=(
            "Obtain the credential json and copy/paste it into this text field."
        ),
    )

    def __str__(self):
        return self.name


class CloudResource(models.Model):
    """The base class of all cloud resource"""

    cloud_credential = models.ForeignKey(
        Credential,
        help_text="Choose the credential to use with this cloud resource",
        on_delete=models.RESTRICT,
    )
    cloud_id = models.CharField(
        max_length=4096,
        help_text="Cloud Resource id (GCP name, etc...)",
        blank=True,
        null=True,
    )
    cloud_state = models.CharField(
        max_length=2,
        choices=CLOUD_RESOURCE_MGMT_STATUS,
        default="nm",
        help_text="Current state of this cloud resource",
    )
    cloud_region = models.CharField(
        max_length=30,
        help_text="The region of this cloud resource",
    )
    cloud_zone = models.CharField(
        max_length=30,
        help_text="The zone of this cloud resource",
        blank=True,
        null=True,
    )

    @property
    def project_id(self):
        if self.cloud_credential:
            cred_info = json.loads(self.cloud_credential.detail)
            return cred_info.get("project_id", None)
        return None

    @property
    def is_managed(self):
        return "m" in self.cloud_state

    @property
    def cloud_status(self):
        raise NameError("Use cloud_state!")

    @cloud_status.setter
    def cloud_status(self, new_status):
        raise NameError("Use cloud_state!")


class VirtualNetwork(CloudResource):
    """Model representing a virtual network (VPC) in the cloud"""

    name = models.CharField(
        max_length=64,
        help_text="Name for the virtual network",
        validators=[
            RFC1035Validator(
                63,
                "VPC Name must be RFC1035 Compliant (lower case, alphanumeric "
                "with hyphens)",
            )
        ],
    )

    def __str__(self):
        return self.name

    def in_use(self):
        return (
            Workbench.objects.filter(subnet__vpc=self)
            .exclude(cloud_state="xm")
            .exists()
            or Cluster.objects.filter(subnet__vpc=self)
            .exclude(cloud_state="xm")
            .exists()
            or Filesystem.objects.filter(subnet__vpc=self)
            .exclude(cloud_state="xm")
            .exists()
        )


class VirtualSubnet(CloudResource):
    """Model representing a subnet in the cloud"""

    name = models.CharField(
        max_length=64,
        help_text="Name for the virtual subnet",
        validators=[
            RFC1035Validator(
                63,
                "Subnet Name must be RFC1035 Compliant (lower case, "
                " alphanumeric with hyphens)",
            )
        ],
    )
    vpc = models.ForeignKey(
        VirtualNetwork,
        related_name="subnets",
        help_text="The VPC to which this subnet belongs",
        on_delete=models.CASCADE,
    )
    cidr = models.CharField(
        max_length=18,
        help_text="CIDR for this subnet",
        validators=[CIDRValidator],
    )

    def __str__(self):
        return f"{self.vpc.name} - {self.name} - {self.cloud_region}"


FILESYSTEM_TYPES = (
    (" ", "none"),
    ("n", "nfs"),
    ("l", "lustre"),
    ("d", "daos"),
    ("b", "beegfs"),
)


class FilesystemImpl(models.IntegerChoices):
    BUILT_IN = 0, "Cluster Built-in"
    GCPFILESTORE = 1, "GCP Filestore"
    IMPORTED = 2, "Imported Filesystem"


class Filesystem(CloudResource):
    """Model representing a file system in the cloud"""

    name = models.CharField(
        max_length=40,
        help_text="Enter a name for the file system",
    )
    internal_name = models.CharField(
        max_length=40,
        help_text="name generated by system (not to be set by user)",
        blank=True,
        null=True,
    )
    subnet = models.ForeignKey(
        VirtualSubnet,
        related_name="filesystems",
        help_text="Subnet within which the Filesystem resides (if any)",
        on_delete=RESTRICT_IF_CLOUD_ACTIVE,
        null=True,
        blank=True,
    )
    vpc = models.ForeignKey(
        VirtualNetwork,
        related_name="filesystems",
        help_text="Network within which the Filesystem resides",
        on_delete=models.SET_NULL,
        null=True,
    )
    impl_type = models.PositiveIntegerField(
        choices=FilesystemImpl.choices,
        blank=False,
    )
    fstype = models.CharField(
        max_length=1,
        choices=FILESYSTEM_TYPES,
        help_text="Type of Filesystem (NFS, Lustre, etc)",
        blank=False,
        default=FILESYSTEM_TYPES[0][0],
    )

    @property
    def fstype_name(self):
        return dict(FILESYSTEM_TYPES).get(self.fstype)

    hostname_or_ip = models.CharField(
        max_length=128,
        help_text="Hostname or IP address of Filesystem server",
        null=True,
        blank=True,
    )

    def __str__(self):
        return self.name


class FilesystemExport(models.Model):
    """Model representing a file system export"""

    # mount -t <fstype> <server_name>:<export_name> /mnt

    filesystem = models.ForeignKey(
        Filesystem, related_name="exports", on_delete=models.CASCADE
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
        help_text="An export from NFS, or name of FS for Lustre, etc.",
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
    """Model representing a mount point"""

    export = models.ForeignKey(
        FilesystemExport,
        related_name="+",
        on_delete=models.CASCADE,
    )

    cluster = models.ForeignKey(
        "Cluster",
        related_name="mount_points",
        on_delete=models.CASCADE,
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
        help_text="Mounts are mounted in numerically increasing order",
        default=0,
    )

    mount_options = models.CharField(
        max_length=128,
        help_text="Mount options (passed to mount -o)",
        blank=True,
    )

    mount_path = models.CharField(
        max_length=4096,
        help_text="Path on which to mount this filesystem",
    )

    def __str__(self):
        return f"{self.mount_path} on {self.cluster}"

class StartupScript(models.Model):
    """Model representing a startup script for custom image build."""

    def __str__(self):
        return self.name

    name = models.CharField(
        max_length=30,
        help_text="Enter a startup script name",
    )
    description = models.TextField(
        max_length=4000,
        help_text="(Optional) description of this startup script",
        blank=True,
        null=True,
    )
    STARTUP_SCRIPT_TYPES = (
        ("shell", "Shell script"),
        ("ansible-local", "Ansible playbook"),
    )
    type = models.CharField(
        max_length=13,
        choices=STARTUP_SCRIPT_TYPES,
        blank=False,
        default="shell",
        help_text="Type of this application installation",
    )
    content = models.FileField(
        upload_to="startup-scripts/",
        help_text="Startup script file."
    )
    owner = models.ForeignKey(
        User,
        related_name="startup_script_owner",
        help_text="Who owns this startup script?",
        on_delete=models.RESTRICT,
    )
    authorised_users = models.ManyToManyField(
        User,
        related_name="startup_script_authorised_users",
        help_text="Select other users authorised to use this startup script",
    )
        
class Image(CloudResource):
    """Model representing a custom node image."""

    name = models.CharField(
        max_length=30,
        help_text="Enter an image name",
        unique=True,
    )

    family = models.CharField(
        max_length=30,
        help_text="Enter you new image family",
        unique=True,
    )

    source_image_project = models.CharField(
        max_length=60,
        help_text="Enter a source image project",
        blank=False,
        default="schedmd-slurm-public",
    )

    source_image_family = models.CharField(
        max_length=60,
        help_text="Enter a source image family",
        blank=False,
        default="schedmd-v5-slurm-22-05-8-rocky-linux-8",
    )

    startup_script = models.ManyToManyField(
        StartupScript,
        help_text="Which startup scripts to use?",
    )

    enable_os_login = models.CharField(
        max_length=5,
        help_text="Enable OS Login during the image creation?",
        choices=(("TRUE", "TRUE"),("FALSE", "FALSE")),
        default="TRUE",
    )

    block_project_ssh_keys = models.CharField(
        max_length=5,
        help_text="Don't use SSH keys in project metadata to create users?",
        choices=(("TRUE", "TRUE"),("FALSE", "FALSE")),
        default="TRUE",
    )
    owner = models.ForeignKey(
        User,
        related_name="image_owner",
        help_text="Who owns this image?",
        on_delete=models.RESTRICT,
    )
    authorised_users = models.ManyToManyField(
        User,
        related_name="image_authorised_users",
        help_text="Select other users authorised to use this image",
    )
    IMAGE_STATUS = (
        ("n", "Image is being newly configured by user"),
        ("c", "Image is being created"),
        ("r", "Image is ready"),
        ("e", "Image creation has failed"),
    )
    status = models.CharField(
        max_length=1,
        choices=IMAGE_STATUS,
        default="n",
        help_text="Status of this image",
    )

    def __str__(self):
        return self.name

class Cluster(CloudResource):
    """Model representing a cluster"""

    name = models.CharField(
        max_length=17,
        help_text="Enter a name for the cluster",
        validators=[
            RFC1035Validator(
                17,
                "Cluster Name must be RFC1035 Compliant (lower case, "
                "alphanumeric with hyphens)",
            )
        ],
    )
    owner = models.ForeignKey(
        User,
        related_name="owner",
        help_text="Who owns this cluster?",
        on_delete=models.RESTRICT,
    )
    subnet = models.ForeignKey(
        VirtualSubnet,
        related_name="clusters",
        help_text="Subnet within which the cluster resides",
        on_delete=RESTRICT_IF_CLOUD_ACTIVE,
        null=True,
        blank=True,
    )
    authorised_users = models.ManyToManyField(
        User,
        related_name="authorised_users",
        help_text="Select other users authorised to use this cluster",
    )
    CLUSTER_STATUS = (
        ("n", "Cluster is being newly configured by user"),
        ("c", "Cluster is being created"),
        ("i", "Cluster is being initialised"),
        ("r", "Cluster is ready for jobs"),
        ("re", "Cluster is reconfiguring"),
        ("s", "Cluster is stopped (can be restarted)"),
        ("t", "Cluster is terminating"),
        ("e", "Cluster deployment has failed"),
        ("d", "Cluster has been deleted"),
    )
    status = models.CharField(
        max_length=2,
        choices=CLUSTER_STATUS,
        default="n",
        help_text="Status of this cluster",
    )
    spackdir = models.CharField(
        max_length=4096,
        verbose_name="Spack directory",
        default="/opt/cluster/spack",
        help_text="Specify where Spack install applications on the cluster",
    )
    shared_fs = models.ForeignKey(
        Filesystem,
        on_delete=RESTRICT_IF_CLOUD_ACTIVE,
        null=True,
        blank=True,
        related_name="+",
    )
    spack_install = models.ForeignKey(
        "ApplicationInstallationLocation",
        on_delete=models.SET_NULL,
        related_name="+",
        null=True,
        blank=True,
    )
    controller_node = models.OneToOneField(
        "ComputeInstance",
        on_delete=models.SET_NULL,
        null=True,
        blank=True,
    )
    controller_instance_type = models.CharField(
        max_length=40,
        help_text="GCP Instance Type name for the controller",
        default="n2-standard-2",
    )
    controller_disk_type = models.CharField(
        max_length=30,
        help_text="GCP Persistent Disk type",
        default="pd-standard",
    )
    controller_disk_size = models.PositiveIntegerField(
        validators=[MinValueValidator(10)],
        help_text="Boot disk size (in GB)",
        default=50,
        blank=True,
    )
    num_login_nodes = models.PositiveIntegerField(
        validators=[MinValueValidator(0)],
        help_text="The number of login nodes to create",
        default=1,
    )
    login_node_instance_type = models.CharField(
        max_length=40,
        help_text="GCP Instance Type name for the login nodes",
        default="n2-standard-2",
    )
    login_node_disk_type = models.CharField(
        max_length=30,
        help_text="GCP Persistent Disk type",
        default="pd-standard",
    )
    login_node_disk_size = models.PositiveIntegerField(
        # login node disk must be large enough to hold the SlurmGCP
        # image: >=50GB
        validators=[MinValueValidator(50)],
        help_text="Boot disk size (in GB)",
        default=50,
        blank=True,
    )
    grafana_dashboard_url = models.CharField(
        max_length=512,
        null=True,
        blank=True,
    )
    login_node_image = models.ForeignKey(
        Image,
        related_name="login_node_image",
        help_text="Select login node image",
        blank=True,
        null=True,
        default=None,
        on_delete=models.SET_NULL
    )
    controller_node_image = models.ForeignKey(
        Image,
        related_name="controller_node_image",
        help_text="Select controller node image",
        blank=True,
        null=True,
        default=None,
        on_delete=models.SET_NULL,
    )
    use_cloudsql = models.BooleanField(
        default=False,
        help_text=(
            "Would you like to use Cloud SQL for Slurm accounting database?"
        ),
    )
    use_bigquery = models.BooleanField(
        default=False,
        help_text=(
            "Would you like to send Slurm accounting data to BigQuery?"
        ),
    )

    def get_access_key(self):
        return Token.objects.get(user=self.owner)

    def total_cost(self, date_range=None):
        # Django won't accept None on a kwarg to ignore it...
        filters = {"cluster": self.id}
        if date_range:
            filters["date_time_submission__range"] = date_range
        jobs = Job.objects.filter(**filters)

        total_cost = Decimal(0)
        for job in jobs:
            total_cost += job.job_cost

        return total_cost

    def total_jobs(self, date_range=None):
        # Django won't accept None on a kwarg to ignore it...
        filters = {"cluster": self.id}
        if date_range:
            filters["date_time_submission__range"] = date_range
        jobs = Job.objects.filter(**filters)

        return len(jobs)

    def __str__(self):
        """String for representing the Model object."""
        return f"Cluster '{self.name}'"


class ComputeInstance(CloudResource):
    """Instance used for compute (vs login controller)"""

    cluster_login = models.ForeignKey(
        Cluster,
        related_name="login_nodes",
        unique=False,
        null=True,
        blank=True,
        on_delete=models.CASCADE,
    )
    internal_ip = models.GenericIPAddressField(
        protocol="IPv4",
        blank=True,
        null=True,
    )
    public_ip = models.GenericIPAddressField(
        protocol="IPv4",
        blank=True,
        null=True,
    )
    instance_type = models.CharField(
        max_length=40,
        help_text="GCP Instance Type name",
    )
    service_account = models.EmailField(
        max_length=512,
        null=True,
        blank=True,
        default="",
    )


class ClusterPartition(models.Model):
    """Compute partition on a cluster"""

    # Define the regex pattern validator
    name_validator = RegexValidator(
        regex=r"^[a-z](?:[a-z0-9]{0,6})$",
        message="Name must start with a lowercase letter and can have up to 7 characters (lowercase letters or digits).",
    )
    # Define the max length validator
    max_length_validator = MaxLengthValidator(7, "Name cannot exceed 7 characters.")
    name = models.CharField(
        max_length=7,
        validators=[name_validator, max_length_validator],
        help_text="Partition name must start with a lowercase letter and can have up to 7 character (lowercase letters or digits).",
    )
    cluster = models.ForeignKey(
        Cluster,
        related_name="partitions",
        on_delete=models.CASCADE,
    )
    machine_type = models.CharField(
        max_length=40,
        help_text="GCP Instance Type name",
    )
    image = models.ForeignKey(
        Image,
        related_name="compute_node_image",
        help_text="Select compute node image",
        blank=True,
        null=True,
        default=None,
        on_delete=models.SET_NULL,
    )
    dynamic_node_count = models.PositiveIntegerField(
        validators=[MinValueValidator(0)],
        help_text="The maximum number of dynamic nodes in the partition",
        default=2,
    )
    static_node_count = models.PositiveIntegerField(
        validators=[MinValueValidator(0)],
        help_text="The number of statically created nodes in the partition",
        default=0,
    )
    enable_placement = models.BooleanField(
        default=False,
        help_text=(
            "Enable Placement Groups (currently only valid for C2, C2D and C3"
            "instances)"
        ),
    )
    enable_hyperthreads = models.BooleanField(
        default=False, help_text="Enable Hyperthreads (SMT)"
    )
    enable_node_reuse = models.BooleanField(
        default=True,
        help_text=(
            "Enable nodes to be re-used for multiple jobs. (Disabled "
            "when Placement Groups are used.)"
        ),
    )
    vCPU_per_node = models.PositiveIntegerField(  # pylint: disable=invalid-name
        validators=[MinValueValidator(1)],
        help_text="The number of vCPU per node of the partition",
        default=1,
    )
    boot_disk_type = models.CharField(
        max_length=30,
        help_text="GCP Persistent Disk type",
        default="pd-standard",
    )
    boot_disk_size = models.PositiveIntegerField(
        validators=[MinValueValidator(49)],
        help_text="Boot disk size (in GB)",
        default=50,
        blank=True,
    )
    GPU_per_node = models.PositiveIntegerField(  # pylint: disable=invalid-name
        validators=[MinValueValidator(0)],
        help_text="The number of GPU per node of the partition",
        default=0,
    )
    GPU_type = models.CharField(  # pylint: disable=invalid-name
        max_length=64, blank=True, default="", help_text="GPU device type"
    )
    additional_disk_count = models.PositiveIntegerField(
        help_text="How many additional disks?",
        default=0,
        blank=True,
    )
    additional_disk_type = models.CharField(
        max_length=30,
        blank=True,
        help_text="Additional Disk type",
        default="pd-standard",
    )
    additional_disk_size = models.PositiveIntegerField(
        help_text="Disk size (in GB)",
        default=375,
        blank=True,
    )
    additional_disk_auto_delete = models.BooleanField(
        default=True,
        help_text=(
            "Automatically delete additional disk when node is deleted?"
        ),
    )

    def __str__(self):
        return self.name

    def clean(self):
        if self.enable_placement and self.enable_node_reuse:
            raise ValidationError("You cannot enable both Placement Groups and Node Reuse simultaneously.") 


class ApplicationInstallationLocation(models.Model):
    """User managed application support"""

    fs_export = models.ForeignKey(
        FilesystemExport,
        on_delete=models.CASCADE,
        help_text="Filestore on which the application resides",
    )
    path = models.CharField(
        max_length=2048,
        help_text="Directory in the filestore where application resides",
    )

    @property
    def filesystem(self):
        return self.fs_export.filesystem

    @property
    def clusters_using(self):
        return Cluster.objects.filter(mount_points__export=self.fs_export)


class Application(models.Model):
    """Model representing a particular binary installation of an application."""

    name = models.CharField(
        max_length=30,
        help_text="Enter an application name",
    )
    description = models.TextField(
        max_length=4000,
        help_text="(Optional) description of this application",
        blank=True,
        null=True,
    )
    version = models.CharField(
        max_length=30,
        help_text="(Optional) which version of this application",
        blank=True,
        null=True,
    )
    # We store both the cluster and the installation location
    # This allows us to track which cluster was used to perform the installation
    cluster = models.ForeignKey(
        Cluster,
        help_text="Which cluster was used to install the application",
        on_delete=models.CASCADE,
    )
    install_loc = models.ForeignKey(
        ApplicationInstallationLocation,
        help_text="Location of the application installation",
        on_delete=models.CASCADE,
        blank=True,
        null=True,
    )
    install_partition = models.ForeignKey(
        ClusterPartition,
        help_text="Cluster partition on which the installation job will be run",
        on_delete=models.RESTRICT,
        blank=True,
        null=True,
    )
    installed_architecture = models.CharField(
        max_length=128,
        help_text="CPU architecture of installed package",
        blank=True,
        null=True,
    )
    load_command = models.CharField(
        max_length=200,
        help_text=(
            "Commands to load the application package, e.g. 'spack load "
            "xxx' or 'module load yyy'"
        ),
        blank=True,
        null=True,
    )
    compiler = models.CharField(
        max_length=40,
        help_text="Which compiler was used to build this application",
        blank=True,
        null=True,
    )
    mpi = models.CharField(
        max_length=40,
        help_text="Which MPI library was this application built against",
        blank=True,
        null=True,
    )
    APPLICATION_INSTALLATION_STATUS = (
        ("n", "Application is being newly configured"),
        ("p", "Application installation is being prepared"),
        ("q", "Application installation is in job queue"),
        ("i", "Application is being installed"),
        ("r", "Application successfully installed and ready to run"),
        ("e", "Application installation completed in error"),
        ("x", "Hosting cluster has been destroyed"),
    )
    status = models.CharField(
        max_length=1,
        choices=APPLICATION_INSTALLATION_STATUS,
        default="n",
        help_text="Status of this application installation",
    )

    def __str__(self):
        """String for representing the Model object."""
        return f"{self.name} - {self.get_status_display()}"

    def total_spend(self, date_range=None):
        # Django won't accept None on a kwarg to ignore it...
        filters = {"application": self.id}
        if date_range:
            filters["date_time_submission__range"] = date_range
        jobs = Job.objects.filter(**filters)

        total_spend = Decimal(0)
        for job in jobs:
            total_spend += job.job_cost

        return total_spend

    def total_jobs(self, date_range=None):
        # Django won't accept None on a kwarg to ignore it...
        filters = {"application": self.id}
        if date_range:
            filters["date_time_submission__range"] = date_range
        jobs = Job.objects.filter(**filters)

        return len(jobs)


class CustomInstallationApplication(Application):
    """Managed non-spack application"""

    install_script = models.CharField(
        max_length=8192,
        help_text="The URL to a an installation script, or the raw script",
    )

    module_name = models.CharField(
        max_length=128,
        help_text="name of module file to install, and load",
        blank=True,
        null=True,
    )

    module_script = models.CharField(
        max_length=8192,
        help_text="environment modules file to install to load application",
        blank=True,
        null=True,
    )


class SpackApplication(Application):
    """Managed Spack-installed application"""

    spack_name = models.CharField(
        max_length=30,
        help_text="Name of the application in Spack",
        blank=True,
        null=True,
    )
    spack_spec = models.CharField(
        max_length=200,
        help_text="Spack spec describing this particular build configuration",
        blank=True,
        null=True,
    )
    spack_hash = models.CharField(
        max_length=32,
        help_text="Hash of the Spack installation of the application package",
        blank=True,
        null=True,
    )


class Benchmark(models.Model):
    """Model representing a benchmark"""

    name = models.CharField(
        max_length=30,
        help_text="Enter a name of this benchmark",
    )
    description = models.TextField(
        max_length=4000,
        help_text="Enter a description of this benchmark",
    )

    def __str__(self):
        """String for representing the Benchmark object."""
        return self.name


class Job(models.Model):
    """Model representing a single run of an application"""

    application = models.ForeignKey(
        Application,
        help_text="Which application installation to use?",
        on_delete=models.RESTRICT,
    )
    cluster = models.ForeignKey(
        Cluster,
        help_text="Which cluster was used for this job",
        on_delete=models.SET_NULL,
        null=True,
    )
    name = models.CharField(
        max_length=40,
        help_text="Enter a job name",
    )
    date_time_submission = models.DateTimeField(
        blank=True,
        null=True,
        auto_now_add=True,
    )
    user = models.ForeignKey(
        User,
        help_text="Who owns this job?",
        on_delete=models.CASCADE,
    )
    partition = models.ForeignKey(
        ClusterPartition,
        help_text="Cluster partition on which the job will be run",
        on_delete=models.CASCADE,
    )
    number_of_nodes = models.PositiveIntegerField(
        validators=[MinValueValidator(1)],
        help_text="The number of nodes to use",
    )
    ranks_per_node = models.PositiveIntegerField(
        validators=[MinValueValidator(1)],
        help_text="The number of MPI ranks per node",
    )
    threads_per_rank = models.PositiveIntegerField(
        validators=[MinValueValidator(1)],
        default=1,
        help_text="The number of threads per MPI rank (for hybrid jobs)",
    )
    wall_clock_time_limit = models.PositiveIntegerField(
        validators=[MinValueValidator(0)],
        default=0,
        help_text="The wall clock time limit of this job (in minutes)",
        blank=True,
        null=True,
    )
    run_script = models.CharField(
        max_length=8192,
        help_text=(
            "The URL to the job script (a shell script or a tarball "
            "containing run.sh). Or the raw script"
        ),
    )
    # adapted from Django's URL validation regex
    ul = "\u00a1-\uffff"
    hostname_re = (
        r"[a-z" + ul + r"0-9](?:[a-z" + ul + r"0-9-]{0,61}[a-z" + ul + r"0-9])?"
    )
    domain_re = r"(?:\.(?!-)[a-z" + ul + r"0-9-]{1,63}(?<!-))*"
    tld_re = (
        r"\."  # dot
        r"(?!-)"  # can't start with a dash
        r"(?:[a-z" + ul + "-]{2,63}"  # domain label
        r"|xn--[a-z0-9]{1,59})"  # or punycode label
        r"(?<!-)"  # can't end with a dash
        r"\.?"  # may have a trailing dot
    )
    host_re = (
        "("
        + hostname_re
        + domain_re
        + tld_re
        + "|"
        + hostname_re
        + "|localhost)"
    )
    ipv4_re = r"(?:25[0-5]|2[0-4]\d|[0-1]?\d?\d)(?:\.(?:25[0-5]|2[0-4]\d|[0-1]?\d?\d)){3}"  # pylint: disable=line-too-long
    cloud_storage_url_regex = re.compile(
        r"^(?:http|https|gs|s3)://"  # schemes
        r"(?:" + ipv4_re + "|" + host_re + ")"
        r"(?::\d{2,5})?"  # port
        r"(?:[/?#][^\s]*)?"  # resource path
        r"\Z",
        re.IGNORECASE,
    )

    cloud_storage_url_validator = RegexValidator(
        cloud_storage_url_regex,
        message="Error validating cloud storage URL",
    )
    # Note: Cannot use URLField here.
    # The Django URLValidator has issues:
    # 1) FieldValidator here doesn't get matched to Form's Field Validator
    # 2) URLValidator doesn't support hostnames without a TLD, so things like:
    # gs://mcbench/foo/bar    Are considered invalid.
    # At some point, a new validator should be written, but not today.
    input_data = models.CharField(
        max_length=200,
        help_text="(Optional) the URL to download input dataset",
        blank=True,
        validators=[cloud_storage_url_validator],
    )
    result_data = models.CharField(
        max_length=200,
        help_text="(Optional) the URL to upload result dataset",
        blank=True,
        validators=[cloud_storage_url_validator],
    )
    JOB_STATUS = (
        ("n", "A new job has been created and is being configured"),
        ("p", "Job is being prepared"),
        ("q", "Job is in a queue"),
        ("d", "Job input dataset is being downloaded from long-term storage"),
        ("r", "Job is running on the cluster"),
        ("u", "Job result dataset is being uploaded to long-term storage"),
        ("c", "Job has completed successfully"),
        ("e", "Job has completed in error"),
    )
    slurm_jobid = models.PositiveIntegerField(
        blank=True,
        null=True,
        help_text="SLURM Job ID",
    )
    status = models.CharField(
        max_length=1,
        choices=JOB_STATUS,
        default="n",
        help_text="Status of this job",
    )
    runtime = models.FloatField(
        help_text="Job run time (in seconds)",  # as reported by scheduler
        blank=True,
        null=True,
    )
    node_price = models.DecimalField(
        max_digits=8,
        decimal_places=3,
        help_text="Node price - hourly rate",
        blank=True,
        null=True,
    )
    job_cost = models.DecimalField(
        max_digits=12,
        decimal_places=3,
        help_text="Total job cost (predicted or actual)",
        blank=True,
        default=0.0,
    )
    result_unit = models.CharField(
        max_length=20,
        help_text="The unit of a key performance indicator",
        blank=True,
        null=True,
    )
    result_value = models.FloatField(
        help_text="The value of a key performance indicator",
        blank=True,
        null=True,
    )
    CLEANUP_OPTIONS = (
        ("a", "Always remove job files"),
        ("e", "Only remove job files on error"),
        ("s", "Only remove job files on success"),
        ("n", "Never remove job files"),
    )
    cleanup_choice = models.CharField(
        max_length=1,
        choices=CLEANUP_OPTIONS,
        default="n",
        help_text="When to remove job files",
    )
    benchmark = models.ForeignKey(
        Benchmark,
        help_text="(Optional) Identify the benchmark this job belongs to",
        on_delete=models.RESTRICT,
        blank=True,
        null=True,
    )

    def __str__(self):
        """String for representing the Model object."""
        return f"#{self.id} - '{self.name}' on {self.application.cluster}"


class Task(models.Model):
    owner = models.ForeignKey(
        User,
        help_text="Who is running the task",
        on_delete=models.RESTRICT,
    )
    title = models.CharField(
        max_length=128,
    )
    data = models.JSONField(
        blank=True,
        null=False,
        default=dict,
    )


class CallbackField(models.TextField):
    """Serializable Python callbacks for cluster management operations"""

    empty_strings_allowed = False
    description = "Serializable Python Callback"

    # N.B the following pickling operations should realistically only fail either
    # during development or as the result of package upgrades.  In any case
    # there is nothing that can be done at runtime to fix the issue so no sense
    # in trying to carefully pick through exception types

    def to_python(self, value):
        if isinstance(value, str):
            try:
                return dill.loads(base64.decodebytes(bytes(value, "utf-8")))
            except Exception as err:  # pylint: disable=broad-except
                logger.exception("Failed to deserialize callback: %s", err)
                return None

        return value

    def from_db_value(self, value, unused_expression, unused_connection):
        try:
            return dill.loads(base64.decodebytes(bytes(value, "utf-8")))
        except Exception as err:  # pylint: disable=broad-except
            logger.exception("Failed to deserialize callback: %s", err)
            return None

    def get_prep_value(self, value):
        value = super().get_prep_value(value)
        if value is None:
            return None

        try:
            return base64.encodebytes(dill.dumps(value)).decode("utf-8")
        except Exception as err:  # pylint: disable=broad-except
            logger.exception("Failed to serialize callback: %s", err)
            return None


class C2Callback(models.Model):
    ackid = models.UUIDField(
        primary_key=True, default=uuid.uuid4, editable=False
    )
    callback = CallbackField()


class GCPFilestoreFilesystem(Filesystem):
    """Managed GCP filestore-based filesystem"""

    FILESTORE_TIER = (
        ("bh", "BASIC_HDD"),
        ("bs", "BASIC_SSD"),
        ("hs", "HIGH_SCALE_SSD"),
        ("en", "ENTERPRISE"),
    )
    capacity = models.PositiveIntegerField(
        validators=[MinValueValidator(1024)],
        help_text="Capacity (in GB) of the filesystem (min of 2660)",
        default=1024,
    )

    performance_tier = models.CharField(
        max_length=2,
        choices=FILESTORE_TIER,
        help_text="Filestore Performance Tier",
    )

    def save(self, *args, **kwargs):
        self.fstype = "n"
        self.impl_type = FilesystemImpl.GCPFILESTORE
        super().save(*args, **kwargs)

    def __str__(self):
        return f"GCP Filestore {self.name}"


FILESYSTEM_IMPL_INFO = {
    FilesystemImpl.BUILT_IN: {"name": "Cluster Built-in", "class": None},
    FilesystemImpl.GCPFILESTORE: {
        "name": "GCP Filestore",
        "class": GCPFilestoreFilesystem,
        "url-key": "filestore",
        "terraform_dir": "gcp_filestore",
    },
    FilesystemImpl.IMPORTED: {
        "name": "Imported Filesystem",
        "class": Filesystem,
        "url-key": "import-fs",
    },
}


class WorkbenchPreset(models.Model):
    """Model representing a vertex AI workbench"""

    name = models.CharField(
        max_length=40,
        primary_key=True,
        help_text="Enter a name for the Workbench Preset",
    )

    machine_type = models.CharField(
        max_length=40,
        default="n1-standard-1",
        help_text="The machine type for this workbench size",
    )

    category = models.CharField(
        max_length=40,
        help_text="Enter the heading this preset should appear under",
    )


class Workbench(CloudResource):
    """Model representing a vertex AI workbench"""

    name = models.CharField(
        max_length=40,
        help_text="Enter a name for the Workbench",
        validators=[
            RFC1035Validator(
                63,
                "Workbench Name must be RFC1035 Compliant (lower-case "
                "alphanumeric with hyphens)",
            )
        ],
    )
    internal_name = models.CharField(
        max_length=40,
        help_text="Workbench name generated by system (not to be set by user)",
        blank=True,
        null=True,
    )
    owner = models.ForeignKey(
        User,
        related_name="workbench_owner",
        help_text="Who owns this Workbench?",
        on_delete=models.RESTRICT,
    )
    subnet = models.ForeignKey(
        VirtualSubnet,
        related_name="workbench_subnet",
        help_text="Subnet within which the workbench resides",
        on_delete=RESTRICT_IF_CLOUD_ACTIVE,
        null=True,
        blank=True,
    )
    attached_cluster = models.ForeignKey(
        Cluster,
        related_name="attached_workbenches",
        help_text="Cluster to which jobs may be submitted",
        on_delete=RESTRICT_IF_CLOUD_ACTIVE,
        null=True,
        blank=True,
    )
    WORKBENCH_STATUS = (
        ("n", "Workbench is being newly configured by user"),
        ("c", "Workbench is being created"),
        ("i", "Workbench is being initialised"),
        ("r", "Workbench is ready"),
        ("s", "Workbench is stopped (can be restarted)"),
        ("t", "Workbench is terminating"),
        ("e", "Workbench deployment has failed"),
        ("d", "Workbench has been destroyed"),
    )
    status = models.CharField(
        max_length=1,
        choices=WORKBENCH_STATUS,
        default="n",
        help_text="Status of this cluster",
    )
    machine_type = models.CharField(
        max_length=40,
        default="n1-standard-1",
        help_text="The machine type for this workbench",
    )
    WORKBENCH_BOOTDISKTYPE = (
        ("PD_STANDARD", "Standard Persistent Disk"),
        ("PD_BALANCED", "Balanced Persistent Disk"),
        ("PD_SSD", "SSD Persistent Disk"),
    )
    boot_disk_type = models.CharField(
        max_length=11,
        choices=WORKBENCH_BOOTDISKTYPE,
        default="PD_STANDARD",
        help_text="Type of storage to be required for notebook boot disk",
    )
    boot_disk_capacity = models.PositiveIntegerField(
        validators=[MinValueValidator(100)],
        help_text="Capacity (in GB) of the filesystem (min of 1024)",
        default=100,
    )
    proxy_uri = models.CharField(max_length=150, blank=True, null=True)
    trusted_user = models.ForeignKey(
        User,
        help_text="Select primary user authorised to use this workbench",
        on_delete=models.RESTRICT,
    )
    # pylint: disable=line-too-long
    WORKBENCH_IMAGEFAMILIES = (
        ("common-cpu-notebooks-ubuntu-2004", "Base Python 3 (with Intel MKL)"),
        ("tf-latest-cpu-ubuntu-2004", "TensorFlow Enterprise (Intel® MKL-DNN/MKL)"),
        ("pytorch-latest-cpu-ubuntu-2004", "PyTorch"),
        ("r-latest-cpu-experimental-ubuntu-2004", "R (Experimental)"),
    )
    # pylint: enable=line-too-long
    image_family = models.CharField(
        max_length=64,
        choices=WORKBENCH_IMAGEFAMILIES,
        default="base-cpu",
        help_text="Select the image family that you wish to use",
    )

    @property
    def get_access_key(self):
        return Token.objects.get(user=self.owner)

    def __str__(self):
        """String for representing the Model object."""
        return f"Workbench '{self.name}'"

class WorkbenchMountPoint(models.Model):
    """Model representing a mount point"""

    export = models.ForeignKey(
        FilesystemExport,
        related_name="+",
        on_delete=models.CASCADE,
    )

    workbench = models.ForeignKey(
        "Workbench",
        related_name="mount_points",
        on_delete=models.CASCADE,
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
        help_text="Mounts are mounted in numerically increasing order",
        default=0,
    )

    mount_path = models.CharField(
        max_length=4096,
        help_text="Path on which to mount this filesystem",
    )

    def __str__(self):
        return f"{self.mount_path} on {self.workbench}"
