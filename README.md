# Qall Daemon

**Execution runtime for Qall workflows.**

Qall Daemon is the execution component of the [Qall](https://github.com/qall-project/qall) framework.

It is responsible for turning Qall computation graphs into running workloads and managing the lifecycle of task and worker runtimes.

## What does it do?

The daemon sits between the Qall computation model and the underlying execution environment.

```text
        Qall computation graph (registry)
                  │
                  ▼
            Qall Daemon
                  │
       ┌──────────┼──────────┐
       ▼          ▼          ▼
      CPU        GPU        QPU
       │          │          │
       └──────────┼──────────┘
                  ▼
             Task / Worker
              runtimes
```

It handles the execution lifecycle of Qall workloads, including:
- retrieving computation definitions
- preparing execution environments
- starting task and worker runtimes
- managing runtime lifecycle
- coordinating execution between Qall components

## Architecture

The daemon exposes interfaces used by the Qall SDK and other Qall components.

The repository contains:

- **Client**: client-side interface to the daemon
- **Server**: daemon service implementation
- **OpenAPI / Protobuf**: service interface definitions

## Role in Qall
The daemon is deliberately separated from the Qall SDK.

The SDK describes **what should be executed**.

The daemon is responsible for **executing it**.

This separation allows the Qall computation model to remain independent from a particular execution environment.

## Project status

Qall Daemon is an **early-stage component of the Qall project** and its APIs are under active development.

See the main [Qall repository](https://github.com/qall-project/qall) for the overall project and usage examples.
