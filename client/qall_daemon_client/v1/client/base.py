# Copyright 2026 Scaleway
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
from __future__ import annotations

from abc import ABC
from typing import Optional

from qall_daemon_client.v1.objects import DaemonInfo, TaskRun, Artifact, WorkerEntry


class DaemonClient(ABC):

    def __enter__(self):
        raise NotImplementedError

    def __exit__(self, exc_type, exc_val, exc_tb):
        raise NotImplementedError

    def get_service_info(self) -> DaemonInfo:
        raise NotImplementedError

    def create_task_run(
        self,
        task_hash: str,
        artifact_hash: Optional[str] = None,
        parent_task_run_id: Optional[str] = None,
    ) -> TaskRun:
        raise NotImplementedError

    def get_task_run(self, task_run_id: str) -> TaskRun:
        raise NotImplementedError

    def create_worker_entry(
        self, worker_hash: str, worker_provider: str, input_format: str
    ) -> WorkerEntry:
        raise NotImplementedError

    def create_artifact(
        self,
        task_run_id: str,
        payload: bytes,
    ) -> Artifact:
        raise NotImplementedError

    def download_artifact(self, artifact_hash: str) -> bytes:
        raise NotImplementedError

    def list_artifacts(self, task_run_id: str) -> list[Artifact]:
        raise NotImplementedError

    def is_up(self) -> bool:
        try:
            return self.get_service_info() is not None
        except:
            return False
