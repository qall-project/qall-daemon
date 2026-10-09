from google.protobuf import empty_pb2 as _empty_pb2
from google.api import annotations_pb2 as _annotations_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import (
    ClassVar as _ClassVar,
    Iterable as _Iterable,
    Mapping as _Mapping,
    Optional as _Optional,
    Union as _Union,
)

DESCRIPTOR: _descriptor.FileDescriptor

class DaemonInfo(_message.Message):
    __slots__ = ("name", "version")
    NAME_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    name: str
    version: str
    def __init__(
        self, name: _Optional[str] = ..., version: _Optional[str] = ...
    ) -> None: ...

class CreateWorkerEntryRequest(_message.Message):
    __slots__ = ("worker_hash", "worker_provider", "input_format")
    WORKER_HASH_FIELD_NUMBER: _ClassVar[int]
    WORKER_PROVIDER_FIELD_NUMBER: _ClassVar[int]
    INPUT_FORMAT_FIELD_NUMBER: _ClassVar[int]
    worker_hash: str
    worker_provider: str
    input_format: str
    def __init__(
        self,
        worker_hash: _Optional[str] = ...,
        worker_provider: _Optional[str] = ...,
        input_format: _Optional[str] = ...,
    ) -> None: ...

class CreateResourceAssignmentRequest(_message.Message):
    __slots__ = ("task_hash", "resource_provider", "resource_name")
    TASK_HASH_FIELD_NUMBER: _ClassVar[int]
    RESOURCE_PROVIDER_FIELD_NUMBER: _ClassVar[int]
    RESOURCE_NAME_FIELD_NUMBER: _ClassVar[int]
    task_hash: str
    resource_provider: str
    resource_name: str
    def __init__(
        self,
        task_hash: _Optional[str] = ...,
        resource_provider: _Optional[str] = ...,
        resource_name: _Optional[str] = ...,
    ) -> None: ...

class CreateTaskRunRequest(_message.Message):
    __slots__ = ("task_hash", "artifact_hash", "provider_credentials")

    class ProviderCredentialsEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(
            self, key: _Optional[str] = ..., value: _Optional[str] = ...
        ) -> None: ...

    TASK_HASH_FIELD_NUMBER: _ClassVar[int]
    ARTIFACT_HASH_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_CREDENTIALS_FIELD_NUMBER: _ClassVar[int]
    task_hash: str
    artifact_hash: str
    provider_credentials: _containers.ScalarMap[str, str]
    def __init__(
        self,
        task_hash: _Optional[str] = ...,
        artifact_hash: _Optional[str] = ...,
        provider_credentials: _Optional[_Mapping[str, str]] = ...,
    ) -> None: ...

class TaskRun(_message.Message):
    __slots__ = ("id", "status")

    class Status(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
        __slots__ = ()
        UNKNOWN: _ClassVar[TaskRun.Status]
        PENDING: _ClassVar[TaskRun.Status]
        RUNNING: _ClassVar[TaskRun.Status]
        DONE: _ClassVar[TaskRun.Status]
        ERROR: _ClassVar[TaskRun.Status]

    UNKNOWN: TaskRun.Status
    PENDING: TaskRun.Status
    RUNNING: TaskRun.Status
    DONE: TaskRun.Status
    ERROR: TaskRun.Status
    ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    id: str
    status: TaskRun.Status
    def __init__(
        self,
        id: _Optional[str] = ...,
        status: _Optional[_Union[TaskRun.Status, str]] = ...,
    ) -> None: ...

class GetTaskRunRequest(_message.Message):
    __slots__ = ("task_run_id",)
    TASK_RUN_ID_FIELD_NUMBER: _ClassVar[int]
    task_run_id: str
    def __init__(self, task_run_id: _Optional[str] = ...) -> None: ...

class CreateArtifactRequest(_message.Message):
    __slots__ = ("task_run_id", "payload")
    TASK_RUN_ID_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    task_run_id: str
    payload: bytes
    def __init__(
        self, task_run_id: _Optional[str] = ..., payload: _Optional[bytes] = ...
    ) -> None: ...

class ListArtifactsRequest(_message.Message):
    __slots__ = ("task_run_id",)
    TASK_RUN_ID_FIELD_NUMBER: _ClassVar[int]
    task_run_id: str
    def __init__(self, task_run_id: _Optional[str] = ...) -> None: ...

class DownloadArtifactRequest(_message.Message):
    __slots__ = ("artifact_hash",)
    ARTIFACT_HASH_FIELD_NUMBER: _ClassVar[int]
    artifact_hash: str
    def __init__(self, artifact_hash: _Optional[str] = ...) -> None: ...

class Artifact(_message.Message):
    __slots__ = ("hash",)
    HASH_FIELD_NUMBER: _ClassVar[int]
    hash: str
    def __init__(self, hash: _Optional[str] = ...) -> None: ...

class ArtifactPayload(_message.Message):
    __slots__ = ("payload",)
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    payload: bytes
    def __init__(self, payload: _Optional[bytes] = ...) -> None: ...

class ListArtifactsResponse(_message.Message):
    __slots__ = ("artifacts",)
    ARTIFACTS_FIELD_NUMBER: _ClassVar[int]
    artifacts: _containers.RepeatedCompositeFieldContainer[Artifact]
    def __init__(
        self, artifacts: _Optional[_Iterable[_Union[Artifact, _Mapping]]] = ...
    ) -> None: ...

class ResourceAssignment(_message.Message):
    __slots__ = ("task_hash", "resource_provider", "resource_name")
    TASK_HASH_FIELD_NUMBER: _ClassVar[int]
    RESOURCE_PROVIDER_FIELD_NUMBER: _ClassVar[int]
    RESOURCE_NAME_FIELD_NUMBER: _ClassVar[int]
    task_hash: str
    resource_provider: str
    resource_name: str
    def __init__(
        self,
        task_hash: _Optional[str] = ...,
        resource_provider: _Optional[str] = ...,
        resource_name: _Optional[str] = ...,
    ) -> None: ...

class WorkerEntry(_message.Message):
    __slots__ = ("worker_hash", "worker_provider", "input_format")
    WORKER_HASH_FIELD_NUMBER: _ClassVar[int]
    WORKER_PROVIDER_FIELD_NUMBER: _ClassVar[int]
    INPUT_FORMAT_FIELD_NUMBER: _ClassVar[int]
    worker_hash: str
    worker_provider: str
    input_format: str
    def __init__(
        self,
        worker_hash: _Optional[str] = ...,
        worker_provider: _Optional[str] = ...,
        input_format: _Optional[str] = ...,
    ) -> None: ...
