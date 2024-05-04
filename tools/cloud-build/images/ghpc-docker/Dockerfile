# Copyright 2023 Google LLC
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

FROM golang:bullseye AS build
WORKDIR /ghpc-build
ARG ref
RUN git clone https://github.com/GoogleCloudPlatform/hpc-toolkit.git &&\
 cd hpc-toolkit &&\
 git checkout ${ref} &&\
 make ghpc &&\
 go install github.com/google/go-licenses@latest &&\
 /go/bin/go-licenses check "./..." &&\
 /go/bin/go-licenses save "./..." --save_path="THIRD_PARTY_NOTICES"

FROM debian:bullseye-slim
COPY --from=build /ghpc-build/hpc-toolkit/ghpc /usr/bin/ghpc
COPY --from=build /ghpc-build/hpc-toolkit/THIRD_PARTY_NOTICES /THIRD_PARTY_NOTICES

ENTRYPOINT [ "ghpc" ]
