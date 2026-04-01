# Copyright 2026 "Google LLC"
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

"""Example training script."""

import logging
import time
import jax
import jax.numpy as jnp
from jax.sharding import NamedSharding, Mesh
from jax.sharding import PartitionSpec as P
import numpy as np
import random

from google_cloud_mldiagnostics import machinelearning_run
from google_cloud_mldiagnostics import metrics
from google_cloud_mldiagnostics import xprof
from google_cloud_mldiagnostics import metric_types


def predict(params, inputs):
  for W, b in params:
    outputs = jnp.dot(inputs, W) + b
    inputs = jnp.maximum(outputs, 0)
  return outputs


def loss(params, batch):
  inputs, targets = batch
  predictions = predict(params, inputs)
  return jnp.mean(jnp.sum((predictions - targets) ** 2, axis=-1))


def init_layer(key, n_in, n_out):
  k1, k2 = jax.random.split(key)
  W = jax.random.normal(k1, (n_in, n_out)) / jnp.sqrt(n_in)
  b = jax.random.normal(k2, (n_out,))
  return W, b


def init_model(key, layer_sizes, batch_size):
  key, *keys = jax.random.split(key, len(layer_sizes))
  params = list(map(init_layer, keys, layer_sizes[:-1], layer_sizes[1:]))

  _, *keys = jax.random.split(key, 3)
  inputs = jax.random.normal(keys[0], (batch_size, layer_sizes[0]))
  targets = jax.random.normal(keys[1], (batch_size, layer_sizes[-1]))

  return params, (inputs, targets)


def main():
  logging.basicConfig(
      level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s"
  )
  logging.info("Starting JAX training job.")
  jax.distributed.initialize()
  logging.info("✅ JAX distributed environment initialized successfully!")

  
  # --- Training loop starts ---
  machinelearning_run(
      name="workload-diagon",
      run_group="test_rungroup",
      project=<project-name>,
      region=<region>,
      gcs_path=<existing-gcs-bucket-path>, #"gs://diagon-xprof-gcs/test"
      on_demand_xprof=False
  )

  loss_jit = jax.jit(loss)
  gradfun = jax.jit(jax.grad(loss))

  layer_sizes = [784, 1024, 1024, 1024, 10]
  batch_size = 1024 * 8
  params, batch = init_model(jax.random.key(0), layer_sizes, batch_size)

  devices = jax.devices()
  mesh = Mesh(np.array(devices), ('batch',))
  
  sharding = NamedSharding(mesh, P("batch"))
  replicated_sharding = NamedSharding(mesh, P())

  batch = jax.device_put(batch, sharding)
  params = jax.device_put(params, replicated_sharding)

  step_size = 1e-5
  prof = xprof()
  prof.start()
  for step in range(1000):
    start_time = time.time()
    grads = gradfun(params, batch)
    params = [
        (W - step_size * dW, b - step_size * db)
        for (W, b), (dW, db) in zip(params, grads)
    ]
    time.sleep(0.5)  # Simulate some extra work
    if (step + 1) % 10 == 0:
      # Here we are not calculating the metrics following their definition strictly. We populate relatively random numbers only for test.
      # Model quality metrics
      metrics.record(metric_types.MetricType.LEARNING_RATE, step_size, step=step + 1)
      metrics.record(metric_types.MetricType.LOSS, float(loss_jit(params, batch)), step=step + 1)
      metrics.record(metric_types.MetricType.GRADIENT_NORM, float(np.sqrt(sum(jnp.vdot(g, g) for p in grads for g in p))), step=step + 1)
      metrics.record(metric_types.MetricType.TOTAL_WEIGHTS, sum(jnp.size(p) for layer in params for p in layer), step=step + 1)
      # Model performance metrics
      metrics.record(metric_types.MetricType.STEP_TIME, (time.time() - start_time)/10, step=step + 1)
      metrics.record(metric_types.MetricType.THROUGHPUT, 1000 * random.normalvariate(0.5, 0.1), step=step + 1)
      metrics.record(metric_types.MetricType.LATENCY, 1 * random.normalvariate(0.5, 0.1), step=step + 1)
      metrics.record(metric_types.MetricType.TFLOPS, 200 * random.uniform(0.5, 1), step=step + 1)
      metrics.record(metric_types.MetricType.MFU, 100 * random.uniform(0.5, 1), step=step + 1)
  prof.stop()

  logging.info("🎉 Training finished successfully!")


if __name__ == "__main__":
  main()
