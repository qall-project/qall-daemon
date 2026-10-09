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
from pathlib import Path

import grpc
import time

from typing import Optional

from google.protobuf import empty_pb2

from qall_daemon_client.v1.client.protobuf.daemon_runtime_api_v1 import (
    daemon_runtime_api_v1_pb2 as pb2,
)
from qall_daemon_client.v1.client.protobuf.daemon_runtime_api_v1 import (
    daemon_runtime_api_v1_pb2_grpc as pb2_grpc,
)
from qall_daemon_client.v1.objects import (
    Artifact,
    DaemonInfo,
    TaskRun,
    TaskRunStatus,
    WorkerEntry,
)

from .base import DaemonClient


class GrpcDaemonClient(DaemonClient):
    """
    gRPC client for Daemon requests.
    """

    def __init__(
        self,
        url: str = "localhost:50053",
        token: Optional[str] = None,
        local_unified_fs: str = "/tmp/qall_unifs",
    ):
        self.__url = url
        self.__channel: Optional[grpc.Channel] = None
        self.__stub: Optional[pb2_grpc.ApiStub] = None
        self.__token = token
        self.__unifs = Path(local_unified_fs)

        if not self.__unifs.exists():
            self.__unifs.mkdir(mode=0o774, parents=True)
        self.__unifs.chmod(0o774)

    def __enter__(self):
        self.connect()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()

    def connect(self):
        self.__channel = grpc.insecure_channel(self.__url)
        self.__stub = pb2_grpc.ApiStub(self.__channel)

    def close(self):
        if self.__channel:
            self.__channel.close()
            self.__channel = None
            self.__stub = None

    def get_service_info(self) -> DaemonInfo:
        request = empty_pb2.Empty()
        response = self.__stub.GetServiceInfo(request)

        return DaemonInfo(name=response.name, version=response.version)

    def create_worker_entry(
        self, worker_hash: str, worker_provider: str, input_format: str
    ) -> WorkerEntry:
        request = pb2.CreateWorkerEntryRequest(
            worker_hash=worker_hash,
            provider=worker_provider,
            input_format=input_format,
        )

        response = self.__stub.CreateWorkerEntry(request)

        return WorkerEntry(
            hash=response.hash,
            provider=response.provider,
            input_format=response.input_format,
        )

    def create_task_run(
        self,
        task_hash: str,
        artifact_hash: Optional[str] = None,
        provider_credentials: Optional[dict] = None,
    ) -> TaskRun:
        request = pb2.CreateTaskRunRequest(
            task_hash=task_hash,
            artifact_hash=artifact_hash or "",
            provider_credentials=provider_credentials or {},
        )
        response = self.__stub.CreateTaskRun(request)

        return TaskRun(id=response.id, status=TaskRunStatus(response.status))

    def get_task_run(self, task_run_id: str) -> TaskRun:
        request = pb2.GetTaskRunRequest(task_run_id=task_run_id)
        response = self.__stub.GetTaskRun(request)

        return TaskRun(id=response.id, status=TaskRunStatus(response.status))

    def wait_for_task_run(
        self,
        task_run_id: str,
        timeout: float = 300.0,
        poll_interval: float = 1.0,
    ) -> TaskRun:
        """
        Blocking call to wait for a task run to end, meaning its status becomes either DONE or ERROR.
        """

        start_time = time.time()
        while time.time() - start_time < timeout:
            task_run = self.get_task_run(task_run_id)

            if task_run.status in (TaskRunStatus.DONE, TaskRunStatus.ERROR):
                return task_run

            time.sleep(poll_interval)
        raise TimeoutError(
            f"Task run {task_run_id} did not complete within {timeout} seconds"
        )

    def create_artifact(
        self,
        task_run_id: str,
        payload: bytes,
    ) -> Artifact:
        request = pb2.CreateArtifactRequest(
            task_run_id=task_run_id,
            payload=payload,
        )

        grpc_metadata = (("x-qall-token", self.__token),)

        response = self.__stub.CreateArtifact(
            request,
            metadata=grpc_metadata,
        )

        return Artifact(hash=response.hash)

    def download_artifact(self, artifact_hash: str) -> bytes:
        request = pb2.DownloadArtifactRequest(artifact_hash=artifact_hash)
        response = self.__stub.DownloadArtifact(request)

        return response.payload

    def list_artifacts(self, task_run_id: str) -> list[Artifact]:
        request = pb2.ListArtifactsRequest(task_run_id=task_run_id)
        response = self.__stub.ListArtifacts(request)

        return [Artifact(hash=a.hash) for a in response.artifacts]
