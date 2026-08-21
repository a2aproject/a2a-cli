# A2A CLI

<div align="center">
   <img src="https://raw.githubusercontent.com/a2aproject/A2A/refs/heads/main/docs/assets/a2a_logo/color/SVG/a2a_color.svg" width="800" alt="Agent2Agent Protocol Logo"/>
   <h3>
       The <strong>A2A CLI</strong> (<code>a2a</code>) is an official tool developed and maintained by the <a href="https://a2a-protocol.org/latest/"><strong>A2A Project Team</strong></a>.
   </h3>
</div>


The **A2A CLI** (`a2a`) is a standardized command-line client for discovering, interacting with, and managing [A2A (Agent2Agent) agents](https://a2a-protocol.org/latest/).

> [!IMPORTANT]
> **Specification in Active Review (v0.2)**  
> We are formalizing and reviewing the **A2A CLI Specification**. Share feedback, questions, and contributions on the specification documents as we finalize v0.2 in **August 2026**.


## About the Project

Several community-driven CLI tools exist across languages, but they vary in coverage and behavior. To prevent fragmentation, this project defines one **standardized, officially supported CLI specification and reference implementation** — built for long-term stability, cross-transport consistency, and community alignment.

The initial codebase builds on the CLI from the [A2A Go SDK](https://github.com/a2aproject/a2a-go/tree/9d95b95445f4208ba77f48a137a278067937adb7#-cli). We will refine and expand it to match the finalized specification.


## Specification & Conformance (Core Focus)

The specification is a standalone document that defines the CLI behavior, command surface, output schemas, and transport rules for `a2a-cli`:

* **[Specification Document (v0.2)](./specification/SPEC.md):** The normative behavior specification covering command taxonomy, output contracts, polling/streaming rules, and exit codes.
* **[Compliance Model (v0.2)](./specification/COMPLIANCE.md):** The requirement registry and verification model for claiming conformance.
* **[Compliance Report Template](./specification/compliance-report.template.yaml):** The standardized machine-readable report for implementations.


## CLI Development Phases & Roadmap

The specification organizes CLI capabilities into three cumulative tiers. We will build and ship them in order:

* **Tier 1 (Core Requirements):** Essential foundation — agent card discovery (`card get`), basic messaging (`send`), task inspection (`task get`), cancellation (`task cancel`), polling, exit codes, and standard text/JSON output contracts.
* **Tier 2 (Standard Features):** Expanded capabilities — task listing (`task list`), real-time event streaming (`task subscribe`), transport auto-negotiation (REST, JSON-RPC, gRPC), push-configuration, and auth management.
* **Tier 3 (Advanced & Ergonomics):** Advanced tooling — interactive terminal chat, push-notification webhook receivers, extended card verification, and mTLS / OpenID Connect authentication.


## How to Contribute & Provide Feedback

We invite review and input from engineers and the broader community:

1. Read the **[Specification Document (`SPEC.md`)](./specification/SPEC.md)**.
2. Review the **[Compliance Checklist (`COMPLIANCE.md`)](./specification/COMPLIANCE.md)**.
3. Open an issue or pull request in this repository to share suggestions, questions, or edge cases.

You can also help us in other ways like sharing how you use the tool. See the **[Contributing guide](CONTRIBUTING.md)** to get started.


## License

The A2A CLI and Specification are open-source and licensed under the [**Apache License 2.0**](LICENSE).
