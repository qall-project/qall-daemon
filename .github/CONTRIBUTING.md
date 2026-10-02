# Contribute to Qall Daemon

Qall Daemon is an open-source component of Qall, developed by Scaleway's R&D teams and released under the Apache 2.0 license.

The Daemon is responsible for executing Qall workflows and managing the task and worker runtimes required to run computations across heterogeneous resources.

Contributions are welcome through GitHub, whether you want to report a bug, improve documentation, suggest an enhancement, or contribute code.

## Topics

- [Reporting Security Issues](#reporting-security-issues)
- [Reporting Issues](#reporting-issues)
- [Suggesting a Feature](#suggesting-a-feature)
- [Contributing Code](#contributing-code)
- [Pull Request Guidelines](#pull-request-guidelines)
- [Development Principles](#development-principles)
- [Community Guidelines](#community-guidelines)

## Reporting Security Issues

At Scaleway, we take security seriously.

If you find a security issue in Qall Daemon, please notify us by sending an email to [security@scaleway.com](mailto:security@scaleway.com).

Please **do not create a GitHub issue** for security vulnerabilities.

We will follow up with you promptly with more information and a plan for remediation.

We currently do not offer a paid security bounty program, but we greatly appreciate your help in making Qall and its surrounding ecosystem more secure.

## Reporting Issues

Bug reports and detailed issue reports are valuable contributions to Qall Daemon.

Before opening a new issue, please check the existing issues to see whether a similar report already exists. If it does, add a 👍 reaction and provide any additional information that may help us investigate.

When reporting an issue, please include as much relevant information as possible, such as:

- Qall Daemon version or commit
- Operating system
- Runtime or deployment environment
- Relevant workflow or task definition
- Resource configuration, when applicable
- Steps to reproduce the issue
- Expected and actual behavior
- Relevant logs or error messages

For issues involving a specific worker, execution backend or resource, please also provide the relevant configuration when possible.

## Suggesting a Feature

We welcome ideas that improve the execution of Qall workflows across heterogeneous infrastructure.

When suggesting a feature, please consider:

- **What problem does it solve?**
- **Who benefits from it?**
- **What would the expected user or developer experience look like?**
- **Does it fit with Qall's execution model?**
- **Can it remain independent from a specific provider or infrastructure?**
- **Does it belong in the Daemon, or would it be better implemented in the SDK, Registry or another component?**

For larger changes, opening an issue first is recommended so the design can be discussed before implementation.

## Contributing Code

### Submit Code

To contribute code:

1. Fork the project.
2. Create a topic branch from the current `main` branch.
3. Make your changes.
4. Add or update tests covering the changes.
5. Update documentation when relevant.
6. Run the relevant checks locally.
7. Push your commits to your fork.
8. Open a pull request against the `main` branch.

Keep changes focused and avoid combining unrelated changes in the same pull request.

For changes affecting execution behavior, worker management, resource handling or public APIs, we recommend discussing the approach in an issue before implementation.

### Pull Request Guidelines

The goal of the pull request process is to make changes easy to review, understand and maintain.

Please:

- **Use a clear pull request title** describing what is being changed.
- **Keep pull requests focused** on a specific change or problem.
- **Include tests** for new or modified behavior.
- **Update documentation** when the change affects execution behavior or public interfaces.
- **Keep the implementation readable** and avoid unnecessary complexity.
- **Explain design decisions** when they are not obvious from the code.
- **Mark work-in-progress pull requests** with `[WIP]` when they are not ready for review.
- **Keep the pull request up to date** with the current `main` branch.

If you are addressing an existing issue, reference it from the pull request description.

Please do not merge `main` into your topic branch. Rebase your branch when necessary to keep it up to date.

Maintainers may request changes, additional tests or documentation before merging.

## Development Principles

Qall Daemon is designed around a few principles that are particularly relevant when contributing:

- **Execution-focused**: the Daemon executes computations and manages their runtime lifecycle. It should not contain application-level workflow logic.
- **Infrastructure agnostic**: avoid coupling the execution model to a specific cloud provider, hardware vendor or infrastructure implementation.
- **Separation of concerns**: keep workflow execution, task runtimes, worker runtimes and infrastructure integrations clearly separated.
- **Heterogeneous execution**: the Daemon must be able to manage workloads across different types of compute resources, including CPUs, GPUs, emulators and QPUs.
- **Isolation**: task and worker runtimes should remain isolated from the Daemon and from each other where appropriate.
- **Reproducibility**: execution should be based on explicit computation objects, resource requirements and runtime environments rather than implicit host configuration.
- **Observability**: execution state, logs and relevant runtime information should remain accessible to users and higher-level Qall components.
- **Composable execution**: the Daemon should execute individual tasks and workflows without imposing unnecessary constraints on how they are composed.

When adding a new capability, consider whether it belongs in the Daemon itself or should instead be implemented in the Qall SDK, Registry, or a provider-specific integration.

## Workers and Execution Backends

Qall Daemon uses workers to connect computations to execution backends.

When contributing worker-related code:

- Keep provider-specific logic inside the appropriate worker or adapter.
- Avoid exposing provider-specific APIs in the core execution model.
- Keep worker lifecycle management separate from the workload being executed.
- Prefer well-defined interfaces between the Daemon and workers.
- Consider both local and remote execution when designing worker integrations.
- Ensure that failures and unavailable resources are handled explicitly.

Workers should provide the execution capabilities required by the Daemon rather than becoming independent schedulers or workflow engines.

## Runtime and Resource Management

Changes affecting runtime or resource management should take into account:

- task lifecycle and failure handling;
- worker lifecycle and cleanup;
- resource allocation and release;
- execution isolation;
- logs and execution state;
- long-running workloads;
- retries and partial execution where applicable.

Resource-specific behavior should remain behind appropriate abstractions whenever possible.

## Community Guidelines

Please be respectful and constructive when participating in the Qall community.

Questions, discussions, ideas and constructive criticism are welcome. When proposing changes, focus on the problem being solved and help maintain a collaborative environment.

Thank you for contributing to **Qall Daemon**!