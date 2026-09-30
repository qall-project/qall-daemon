import grpc
import time
from google.protobuf import empty_pb2
from google.protobuf.json_format import MessageToDict

from qall_daemon_client.v1.client.protobuf.daemon_runtime_api_v1 import (
    daemon_runtime_api_v1_pb2 as pb2,
)
from qall_daemon_client.v1.client.protobuf.daemon_runtime_api_v1 import (
    daemon_runtime_api_v1_pb2_grpc as pb2_grpc,
)


def run():
    server_address = "127.0.0.1:50053"
    print(f"Connexion au daemon sur {server_address}...")

    with grpc.insecure_channel(server_address) as channel:
        client = pb2_grpc.ApiStub(channel)

        try:
            request = empty_pb2.Empty()

            print("\nEnvoi de la requête GetServiceInfo...")
            response = client.GetServiceInfo(request)

            print("\n--- Réponse du Daemon reçue avec succès ---")
            print(f"Nom du service : {response.name}")
            print(f"Version        : {response.version}")
            print("------------------------------------------")

        except grpc.RpcError as e:
            print(f"\nErreur lors de l'appel gRPC : {e.code()}")
            print(f"Détails : {e.details()}")

        try:
            target_hash = "zdpuAzqLuf5oJ9PCKdXPDRzJdU83HsEmXpPszHnAbN1Y2Dr2m"

            request = pb2.CreateTaskRunRequest(task_hash=target_hash)
            print("\nEnvoi de la requête CreateTask...")
            response = client.CreateTaskRun(request)

            print("\n--- Réponse du Daemon reçue avec succès ---")
            print(f"Task run id : {response.id}")
            task_run_id = response.id
            print("------------------------------------------")

            try:
                time.sleep(5)
                request = pb2.GetTaskRunRequest(task_run_id=task_run_id)
                print("\nEnvoi de la requête GetTaskRun...")
                response = client.GetTaskRun(request)

                task_data = MessageToDict(response, use_integers_for_enums=False)
                print("\n--- Réponse du Daemon reçue avec succès ---")
                print(f"Task run id : {response.id}")
                print(f"Task status : {task_data['status']}")
                print("------------------------------------------")

            except grpc.RpcError as e:
                print(f"\nErreur lors de l'appel gRPC : {e.code()}")
                print(f"Détails : {e.details()}")

        except grpc.RpcError as e:
            print(f"\nErreur lors de l'appel gRPC : {e.code()}")
            print(f"Détails : {e.details()}")

        try:
            payload = b"test"
            task_run_id = "2efbdfb3-bd3f-46cd-8d23-aeeb4e52a955"
            request = pb2.CreateArtifactRequest(
                task_run_id=task_run_id, payload=payload
            )
            auth_metadata = [("x-qall-token", "8818d5e8-5e74-4268-9ce5-5c6ad9fc2708")]
            print("\nEnvoi de la requête CreateArtifact...")
            response = client.CreateArtifact(request, metadata=auth_metadata)

            print("\n--- Réponse du Daemon reçue avec succès ---")
            print(f"Payload hash : {response.hash[:8]}")
            print("------------------------------------------")

        except grpc.RpcError as e:
            print(f"\nErreur lors de l'appel gRPC : {e.code()}")
            print(f"Détails : {e.details()}")
            return

        try:
            request = pb2.DownloadArtifactRequest(artifact_hash=response.hash)

            print("\nEnvoi de la requête DownloadArtifact...")
            response = client.DownloadArtifact(request)

            print("\n--- Réponse du Daemon reçue avec succès ---")
            print(f"Payload : {response.payload}")
            print("------------------------------------------")

        except grpc.RpcError as e:
            print(f"\nErreur lors de l'appel gRPC : {e.code()}")
            print(f"Détails : {e.details()}")

        try:
            request = pb2.ListArtifactsRequest(task_run_id=task_run_id)

            print("\nEnvoi de la requête ListArtifacts...")
            response = client.ListArtifacts(request)

            print("\n--- Réponse du Daemon reçue avec succès ---")
            print(f"Nombre d'artifacts: {len(response.artifacts)}")
            print(f"[]artifacts :")
            for i, art in enumerate(response.artifacts, 1):
                print(f"  [{i}] Hash: {art.hash}")
            print("------------------------------------------")

        except grpc.RpcError as e:
            print(f"\nErreur lors de l'appel gRPC : {e.code()}")
            print(f"Détails : {e.details()}")


if __name__ == "__main__":
    run()
