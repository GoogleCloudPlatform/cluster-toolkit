# Copyright 2024 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import pytest


# test the chunking logic if not the BigQuery loads themselves
@pytest.mark.parametrize("num_jobs_to_load", (0, 11, 10001, 51131, 104321))
def test_chunked_bq_load(num_jobs_to_load: int):
    BQ_MAX_ROW_LOAD_SIZE = 10000
    jobs = [i + 1 for i in range(num_jobs_to_load)]
    num_batches = (len(jobs) // BQ_MAX_ROW_LOAD_SIZE) + 1
    print(num_batches)
    load_cache = []
    if jobs:
        start_job_idx = 0
        end_job_idx = BQ_MAX_ROW_LOAD_SIZE
        for _ in range(num_batches):
            load_cache.append(jobs[start_job_idx:end_job_idx])
            start_job_idx = end_job_idx
            end_job_idx += BQ_MAX_ROW_LOAD_SIZE
    if jobs:
        assert (
            sum([sum(x) for x in load_cache])
            == num_jobs_to_load * (num_jobs_to_load + 1) // 2
        )
    else:
        assert sum([sum(x) for x in load_cache]) == 0
