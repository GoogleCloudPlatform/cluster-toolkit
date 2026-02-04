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
""" benchmarks.py """
from django.db.models import Count
from django.urls import reverse_lazy
from django.views import generic

from ..forms import BenchmarkForm
from ..models import Benchmark
from ..models import Job
from ..permissions import SuperUserRequiredMixin

# list views


class BenchmarkListView(SuperUserRequiredMixin, generic.ListView):
    """Custom ListView for Benchmark model"""

    model = Benchmark
    template_name = "benchmark/list.html"

    def get_context_data(self, *args, **kwargs):
        context = super().get_context_data(*args, **kwargs)
        context["navtab"] = "benchmark"
        # query the counts of each benchmark and add that information to it
        jobs = (
            Job.objects.all()
            .values("benchmark")
            .annotate(total=Count("benchmark"))
        )
        list_jobs = list(jobs)
        jobs_dict = {}
        for job in list_jobs:
            if job["benchmark"] is not None:
                jobs_dict[job["benchmark"]] = job["total"]
        for benchmark in context["benchmark_list"]:
            try:
                benchmark.count = jobs_dict[benchmark.id]
            except KeyError:
                benchmark.count = 0
        return context


# detail views


class BenchmarkDetailView(SuperUserRequiredMixin, generic.DetailView):
    """Custom DetailView for Benchmark model"""

    model = Benchmark
    template_name = "benchmark/detail.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "benchmark"
        jobs = Job.objects.filter(benchmark=self.kwargs["pk"])
        context["jobs"] = jobs
        return context


# create/update views


class BenchmarkCreateView(SuperUserRequiredMixin, generic.CreateView):
    """Custom CreateView for Benchmark model"""

    success_url = reverse_lazy("benchmarks")
    template_name = "benchmark/create_form.html"
    form_class = BenchmarkForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "benchmark"
        return context
