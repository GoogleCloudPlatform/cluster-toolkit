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

from typing import Any
from pathlib import Path
import shutil
import pickle

import logging
log = logging.getLogger()

# Can't reuse tool from util.py to avoid circular dependencies
# TODO: break down util.py for better modularity.
def _chown_slurm(path: Path) -> None:
    shutil.chown(path, user="slurm", group="slurm")

class FileCache:
    def __init__(self, path: Path):
        self.path = path
    
    def get(self, key: str) -> Any | None:
        p = self.path / key
        if not p.exists():
            return None
        
        try:
            with p.open("rb") as f:
                return pickle.load(f)
            
        except Exception as e:
            log.warning(f"Failed to read cached value at {p}: {e}")
            return None
        
    def set(self, key: str, data: Any) -> None:
        p = self.path / key
        
        try:
            # Create & chown before writing to minimize chances
            # of ending up with root-owned corrupted file that can't be cleaned up
            # TODO: restrict usage of cache by root to avoid all this complexity
            # or have a cache per user.
            p.touch(exist_ok=True)
            _chown_slurm(p)
            with p.open("wb") as f:
                pickle.dump(data, f)
            
        except Exception as e:
            log.warning(f"Failed to write cached value at {p}: {e}")
    

class NoCache:
    def get(self, key: str) -> Any:
        log.warning("No cache used")
        return None
    
    def set(self, key: str, data: Any) -> None:
        log.warning("No cache used")


def cache(name: str) -> FileCache | NoCache:
    try:
        path = Path("/tmp/slurm_gcp_cache/") / name
        if not path.exists():
            path.mkdir(exist_ok=True, parents=True)
            _chown_slurm(path)
        return FileCache(path)
    except:
        log.exception(f"Failed to create cache, fallback to NoCache")
        return NoCache()
