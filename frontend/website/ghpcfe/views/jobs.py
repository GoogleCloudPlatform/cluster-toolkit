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

""" jobs.py """

from decimal import Decimal
from asgiref.sync import sync_to_async
from rest_framework import viewsets
from rest_framework.permissions import IsAuthenticated
from django.contrib.auth.mixins import LoginRequiredMixin
from django.contrib.auth.views import redirect_to_login
from django.contrib import messages
from django.http import HttpResponseRedirect, JsonResponse
from django.urls import reverse, reverse_lazy
from django.views import generic
from django.shortcuts import get_object_or_404
from ..permissions import SuperUserRequiredMixin
from ..models import Application, Job, Role, Cluster
from ..serializers import JobSerializer
from ..forms import JobForm
from ..cluster_manager import c2, cloud_info, utils
from .asyncview import BackendAsyncView
from rest_framework.authtoken.models import Token
from .view_utils import GCSFile, StreamingFileView
import logging
logger = logging.getLogger(__name__)


class JobListView(LoginRequiredMixin, generic.ListView):
    """ Custom ListView for Job model """
    #model = Job  - note this equals to setting the queryset to Job.objects.all()
    template_name = 'job/list.html'

    def get_queryset(self):
        jobs = Job.objects.filter(user=self.request.user) # user only sees its own jobs
        roles = []
        for role in list(self.request.user.roles.all()):
            roles.append(role.id)
        if Role.CLUSTERADMIN in roles:
            jobs = Job.objects.all() # admin gets to see everything
        return jobs

    def get_context_data(self, *args, **kwargs):
        loading = 0
        jobs = self.get_queryset()
        for job in jobs:
            if (job.status == 'p' or job.status == 'q' or job.status == 'd' or job.status == 'r' or job.status == 'u'):
                loading = 1
                break
        context = super().get_context_data(*args, **kwargs)
        context['loading'] = loading
        context['navtab'] = 'job'
        return context


class JobDetailView(LoginRequiredMixin, generic.DetailView):
    """ Custom DetailView for Job model """
    model = Job
    template_name = 'job/detail.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'job'
        return context


class JobCreateView(LoginRequiredMixin, generic.ListView):
    """ Custom CreateView for Job model """

    template_name = 'job/select_cluster.html'
    model = Cluster

    def get_queryset(self):
        # Select clusters which are valid for this application
        app = get_object_or_404(Application, pk=self.kwargs['app'])
        if app.install_loc:
            return app.install_loc.clusters_using.all()
        else:
            # QuerySet of just our cluster
            return Cluster.objects.filter(id=app.cluster.id)

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        application = get_object_or_404(Application, pk=self.kwargs['app'])

        context['application'] = application
        context['navtab'] = 'job'
        return context

    def render_to_response(self, context, **response_kwargs):
        if len(self.object_list) == 1:
            cluster_id = self.object_list[0].id
            return HttpResponseRedirect(reverse('job-create-2', kwargs={'app': self.kwargs['app'], 'cluster': cluster_id}))
        else:
            return super().render_to_response(context, **response_kwargs)


    def post(self, request, app):
        app = get_object_or_404(Application, pk=app)
        cluster_id = request.POST.get("cluster")
        return HttpResponseRedirect(reverse('job-create-2', kwargs={'app': self.kwargs['app'], 'cluster': cluster_id}))


class JobCreateView2(LoginRequiredMixin, generic.CreateView):
    """ Custom CreateView for Job model """

    template_name = 'job/create_form.html'
    form_class = JobForm

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.user = self.request.user

        # Can't trust client side input for these... (bad user, no cookie)
        # self.object.node_price = self.request.POST.get('node_price')
        # self.object.job_cost = self.request.POST.get('job_cost')
        cluster = self.object.cluster
        instance_type = self.object.partition.machine_type

        try:
            node_price_float = cloud_info.get_instance_pricing("GCP",
                                                       cluster.cloud_credential.detail,
                                                       cluster.cloud_region,
                                                       cluster.cloud_zone,
                                                       instance_type)
            self.object.node_price = Decimal(node_price_float)
            print("Got api price {}".format(self.object.node_price))
        except Exception as err:
            form.add_error(None, "Error: Pricing API Unavailable. Please retry in a few moments {}".format(err))
            return self.form_invalid(form)

        self.object.job_cost = self.object.node_price * self.object.number_of_nodes * self.object.wall_clock_time_limit/Decimal(60)


        if self.object.user.quota_type == "d":
            form.add_error(None, "Error: Cannot submit job. User quota disabled")
            return self.form_invalid(form)
        if self.object.user.quota_type == "l":
            quota_remaining = self.object.user.quota_amount - self.object.user.total_spend()
            # Fudge to nearest cent to avoid "apparently equal" issues in user display
            if self.object.job_cost > (quota_remaining - Decimal(0.005)):
                form.add_error(None,
                               "Error: Insufficient quota remaining (have ${:0.2f}, "
                               "job would require ${:0.2f}"
                               "".format(quota_remaining, self.object.job_cost))
                return self.form_invalid(form)

        self.object.save()
        return HttpResponseRedirect(self.get_success_url())


    def get_initial(self):
        cluster = get_object_or_404(Cluster, pk=self.kwargs['cluster'])
        return {'cluster': cluster,
                'application': Application.objects.get(pk=self.kwargs['app']),
                'wall_clock_time_limit': 120}

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        application = get_object_or_404(Application, pk=self.kwargs['app'])
        cluster = get_object_or_404(Cluster, pk=self.kwargs['cluster'])

        context['user_quota_type'] = self.request.user.quota_type
        context['user_quota_remaining'] = self.request.user.quota_amount - self.request.user.total_spend()

        context['application'] = application
        context['cluster'] = cluster
        context['navtab'] = 'job'
        return context

    def get_success_url(self):
        return reverse('backend-job-run', kwargs={'pk': self.object.pk})


class JobRerunView(LoginRequiredMixin, generic.CreateView):
    """ Custom CreateView for rerunning job based on existing job """

    template_name = 'job/rerun_form.html'
    form_class = JobForm

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.user = self.request.user

        # Can't trust client side input for these... (bad user, no cookie)
        # self.object.node_price = self.request.POST.get('node_price')
        # self.object.job_cost = self.request.POST.get('job_cost')
        cluster = self.object.cluster
        instance_type = self.object.partition.machine_type

        try:
            node_price_float = cloud_info.get_instance_pricing("GCP",
                                                       cluster.cloud_credential.detail,
                                                       cluster.cloud_region,
                                                       cluster.cloud_zone,
                                                       instance_type)
            self.object.node_price = Decimal(node_price_float)
            print("Got api price {}".format(self.object.node_price))
        except Exception as err:
            form.add_error(None, "Error: Pricing API Unavailable. Please retry in a few moments {}".format(err))
            return self.form_invalid(form)

        self.object.job_cost = self.object.node_price * self.object.number_of_nodes * self.object.wall_clock_time_limit/Decimal(60)


        if self.object.user.quota_type == "d":
            form.add_error(None, "Error: Cannot submit job. User quota disabled")
            return self.form_invalid(form)
        if self.object.user.quota_type == "l":
            quota_remaining = self.object.user.quota_amount - self.object.user.total_spend()
            # Fudge to nearest cent to avoid "apparently equal" issues in user display
            if self.object.job_cost > (quota_remaining - Decimal(0.005)):
                form.add_error(None,
                               "Error: Insufficient quota remaining (have ${:0.2f}, "
                               "job would require ${:0.2f}"
                               "".format(quota_remaining, self.object.job_cost))
                return self.form_invalid(form)
        self.object.save()
        return HttpResponseRedirect(self.get_success_url())

    def get_initial(self):
        initial = super().get_initial()
        initial = initial.copy()
        existing_job = Job.objects.get(pk=self.kwargs['job'])
        initial['application'] = existing_job.application
        initial['cluster'] = existing_job.cluster
        initial['partition'] = existing_job.partition
        initial['number_of_nodes'] = existing_job.number_of_nodes
        initial['ranks_per_node'] = existing_job.ranks_per_node
        initial['threads_per_rank'] = existing_job.threads_per_rank
        initial['wall_clock_time_limit'] = existing_job.wall_clock_time_limit
        initial['input_data'] = existing_job.input_data
        initial['result_data'] = existing_job.result_data
        initial['run_script'] = existing_job.run_script
        initial['benchmark'] = existing_job.benchmark
        return initial

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        job = Job.objects.get(pk=self.kwargs['job'])
        application = job.application
        cluster = application.cluster
        run_script = job.run_script
        if run_script.startswith("#!"):
            run_script_type = 'raw'
        else:
            run_script_type = 'url'

        context['user_quota_type'] = self.request.user.quota_type
        context['user_quota_remaining'] = self.request.user.quota_remaining()

        context['application'] = application
        context['cluster'] = cluster
        context['navtab'] = 'job'
        context['run_script_type'] = run_script_type
        context['run_script'] = run_script
        return context

    def get_success_url(self):
        return reverse('backend-job-run', kwargs={'pk': self.object.pk})


class JobUpdateView(LoginRequiredMixin, generic.UpdateView):
    """ Custom UpdateView for Job model """

    model = Job

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'job'
        return context


class JobDeleteView(SuperUserRequiredMixin, generic.DeleteView):
    """ Custom DeleteView for Job model """

    # Note on SuperUserRequiredMixin use here:
    # Current cost management model means spend is tied to job records users
    # deleting their own jobs would therefore allow them to delete their spend

    model = Job
    success_url = reverse_lazy('jobs')
    template_name = 'job/confirm_delete.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'job'
        return context

class JobLogFileView(StreamingFileView):
    bucket = utils.load_config()['server']['gcs_bucket']
    valid_logs = [
        {"title": "Job Output", "type": GCSFile, "args": (bucket, "stdout")},
        {"title": "Job Error Log", "type": GCSFile, "args": (bucket, "stderr")},
        {"title": "Job Submit Script", "type": GCSFile, "args": (bucket, "submit.sh")},
    ]

    def _create_FileInfoObject(self, logFileInfo, *args, **kwargs):
        return logFileInfo["type"](*logFileInfo["args"], *args, **kwargs)

    def get_file_info(self):
        logid = self.kwargs.get('logid', -1)
        job_id = self.kwargs.get('pk')
        job = get_object_or_404(Job, pk=job_id)
        cluster_id = job.application.cluster.id
        bucket_prefix = f"clusters/{cluster_id}/jobs/{job.id}"

        entry = self.valid_logs[logid]
        extraArgs = [bucket_prefix]
        return self._create_FileInfoObject(entry, *extraArgs)

class JobLogView(generic.DetailView):
    """ View to display job log files """

    model = Job
    template_name = "job/log.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['log_files'] = [ { "id": n, "title": entry["title"] }
            for n, entry in enumerate(JobLogFileView.valid_logs)
        ]
        context['navtab'] = 'job'
        return context


# For APIs

class JobViewSet(viewsets.ModelViewSet):
    """ Custom ModelViewSet for Job model """
    permission_classes = (IsAuthenticated,)
    queryset = Job.objects.all().order_by('name')
    serializer_class = JobSerializer


# Other supporting views

class BackendJobRun(LoginRequiredMixin, generic.View):
    def get(self, request, pk):
        job = get_object_or_404(Job, pk=pk)
        job.status = 'p'
        job.save()
        cluster_id = job.cluster.id

        try:
            user_uid = job.user.socialaccount_set.first().uid
        except AttributeError:
            if job.user.is_superuser:
                user_uid = '0'
            else:
                # User doesn't have a Google SocialAccount.
                messages.error(request, "You are not signed in with a Google Account. This is required for job submission.")
                job.status = 'n'
                return HttpResponseRedirect(reverse('job-detail', kwargs={'pk':pk}))


        def response(message):
            if message.get('cluster_id') != cluster_id:
                logger.error(f"Cluster ID Mis-match to Callback!  Expected {pk}, Received {message.get('cluster_id')}")
            if message.get('job_id') != pk:
                logger.error(f"Job ID Mis-match to Callback!  Expected {pk}, Received {message.get('job_id')}")


            job = Job.objects.get(pk=pk)
            job.status = message['status']
            logger.info(f"Processing job message, id %d, status %s", pk, job.status)

            if 'slurm_job_id' in message and not job.slurm_jobid:
                job.slurm_jobid = message['slurm_job_id']

            if job.status in ['c', 'e']:
                job.runtime = message.get('job_runtime', None)
                job.result_unit = message.get('result_unit', '')
                job.result_value = message.get('result_value', None)
                job.job_cost = job.number_of_nodes * job.runtime/Decimal(3600) * job.node_price
            job.save()


        message_data = {
            'job_id': job.id,
            'login_uid': user_uid,
            'run_script': job.run_script, # TODO: Should this be base64 encoded?
            'nNodes': job.number_of_nodes,
            'partition': job.partition.name,
        }
        if job.application.load_command:
            message_data['load_command'] = job.application.load_command
        if job.ranks_per_node:
            message_data['ranksPerNode'] = job.ranks_per_node
        if job.threads_per_rank:
            message_data['threadsPerRank'] = job.threads_per_rank
        if job.wall_clock_time_limit:
            message_data['wall_limit'] = job.wall_clock_time_limit
        if job.input_data:
            message_data['input_data'] = job.input_data
        if job.result_data:
            message_data['result_data'] = job.result_data

        c2.send_command(cluster_id, 'RUN_JOB', onResponse=response, data=message_data)
        messages.success(request, "Job sent to Cluster")
        return HttpResponseRedirect(reverse('job-detail', kwargs={'pk':pk}))
