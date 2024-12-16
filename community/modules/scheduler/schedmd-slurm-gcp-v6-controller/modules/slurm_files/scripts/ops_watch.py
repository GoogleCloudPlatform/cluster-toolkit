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

from typing import List, Optional

import json

import logging
from dataclasses import dataclass, asdict
from pathlib import Path

import util

log = logging.getLogger()

SUPPORTED_OPERATION_TYPES = frozenset({"delete"})

@dataclass(frozen=True)
class _Record:
  # common fields
  name: str
  type: str

  region: Optional[str] = None
  zone: Optional[str] = None

  # operation-type specific fields
  nodes: Optional[List[str]] = None

  @classmethod
  def from_json(cls, jo: dict) -> "_Record":
    return cls(**jo)
  
  def to_json(self) -> dict:
    return {k: v for k, v in asdict(self).items() if v is not None}
  
  @classmethod
  def from_op(cls, op: dict, **extra) -> "_Record":
    base = dict(
      name = op["name"],
      type = op["operationType"])
    if "region" in op:
      base["region"] = util.trim_self_link(op["region"])
    if "zone" in op:
      base["zone"] = util.trim_self_link(op["zone"])
    return cls.from_json({**base, **extra})

def _records_dir() -> Path:
  return Path("/var/spool/slurm_gcp/watched_ops") # !!! create it

def _record_path(r: _Record) -> Path:
  return _records_dir() / f"{r.name}.json"

def _list_records() -> List[_Record]:
  res = []
  for p in _records_dir().glob("*.json"):
    try:
      jo = json.loads(p.read_text())
      res.append(_Record.from_json(jo))
    except:
      log.exception(f"Failed to read {p}")
  return res

def _add_record(r: _Record) -> None:
  path = _record_path(r)
  assert not path.exists(), f"{path}"
  # No concern about reading partial writes, 
  # since json deserialization would simply fail
  path.write_text(json.dumps(r.to_json()))

def _remove_record(r: _Record) -> None:
  _record_path(r).unlink(missing_ok=False)

def _get_op_req(r: _Record, lkp: util.Lookup) -> object:
  """
  Queries the state of operation.
  NOTE: it DOES NOT "wait" for operation.
  """
  if r.zone:
    return lkp.compute.zoneOperations().get(project=lkp.project, zone=r.zone, operation=r.name)
  elif r.region:
    return lkp.compute.regionOperations().get(project=lkp.project, region=r.region, operation=r.name)
  raise NotImplementedError("GlobalOperations are not supported")


def _sync_delete_op(r: _Record, lkp: util.Lookup) -> None:
  """
  Processes VM delete-operation.
  If operation is still running - do nothing
  If operation failed - log error & remove op from watch list
  If operation is done - remove op from watch list do nothing
  
  To avoid querying status for each op individually, use list of VM instances as 
  a source of data. Don't query op for instance X if instance X is not present 
  (presumably deleted).
  NOTE: This optimization can lead to false-positives -
  absence of error-logs in case op failed, but VM got deleted by other means.
  """
  assert len(r.nodes) == 1 and r.type == "delete", f"{r}"
  node = r.nodes[0]
  inst = lkp.instance(node)

  if not inst:
    log.debug(f"Stop watching op {r.name}, VM {node} appears to be deleted")
    return _remove_record(r) # potentially false-positive

  log.debug(f"Watching delete-instance op={r.name}. VM {node} status={inst.status}")
  if inst.status == "TERMINATED":
    log.debug(f"Stop watching op {r.name}, VM {node} is TERMINATED")
    return _remove_record(r) # potentially false-positive

  if inst.status == "STOPPING":
    log.debug(f"Skipping op {r.name}, VM {node} is STOPPING")
    return # try later

  op = _get_op_req(r, lkp).execute() # don't handle exceptions, it would be logged and re-tried
  if op["status"] != "DONE":
    log.debug(f"Watching op {r.name} is still not done ({op['status']})")
    return # try later
  
  if "error" in op:
    log.error(f"Operation {r.name} to delete {node} finished with error: {op['error']}")
  else:
    log.debug(f"Operation {r.name} to delete {node} successfully finished")
  return _remove_record(r)

def _sync_deletes(records: List[_Record], lkp: util.Lookup) -> None:
  log.info(f"Processing {len(records)} delete-instance operations")
  
  for r in records:
    try:
      _sync_delete_op(r, lkp)
    except Exception:
      log.exception(f"Failed to process {r}")
      # DO NOT skip others ops processing

def sync_ops(lkp: util.Lookup) -> None:
  records = _list_records()
  for t, records in util.groupby_unsorted(records, lambda r: r.type):
    if t == "delete":
      _sync_deletes(list(records), lkp)
    else:
      log.error(f"Unknown type {t} for {len(records)} operations")

def watch_delete_op(op: dict, node: str) -> None:
  assert op["operationType"] == "delete"
  _add_record(_Record.from_op(op, nodes=[node]))
