# Copyright 2026 Scaleway
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
from dataclasses import dataclass, field


@dataclass(frozen=True)
class DataloaderEntry:
    """Dataloader declaration for one task of a workflow run.

    Sent on the root ``CreateTaskRunRequest`` of the run; the daemon indexes
    the entries per ``task_hash`` so each started container only gets its own.
    ``credentials`` are the plaintext resolved secrets the task's container
    needs to bind the dataloader.
    """

    task_hash: str
    name: str
    src: str
    dst: str
    credentials: dict = field(default_factory=dict)
