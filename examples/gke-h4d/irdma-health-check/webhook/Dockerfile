# Copyright 2025 Google LLC
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

# Use a multi-stage build to create a small, secure image.
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy the Go module files and download dependencies.
COPY go.mod go.sum ./
RUN ls -l /app
RUN go mod download

# Copy the source code and build the binary.
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /webhook

# Create the final, minimal image.
FROM alpine:latest

# Copy the compiled binary from the builder stage.
COPY --from=builder /webhook /webhook

# The webhook will listen on port 8443 for TLS traffic.
EXPOSE 8443

# Run the webhook binary.
ENTRYPOINT ["/webhook"]
