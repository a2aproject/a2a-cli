### Design Principles

This document briefly state the guiding principles for the architecture and code organization of the `a2a`
CLI. The CLI is a **thin orchestration layer over the `a2a-go` SDK**. It owns argument parsing, presentation, and process lifecycle, and the SDK owns the protocol. The command layer is kept thin with logic pushed into small, single-responsibility packages.

1. The CLI package stays thin; logic lives in domain packages. Keep the CLI package clean, create a file-per-command. Try extracting logic into a different package where it can be tested separately.

2. File-per-command, with grouping files for parents. Each leaf command lives in its own file exposing a command constructor. Parent commands are grouping-only: they construct the parent and attach subcommands. `root.go` assembles the whole tree.

3. Construction by injection instead of global state. Commands are built by functions that receive their dependencies. `newRootCmd(cfg, poller)` takes the poller as a parameter so `Execute()` can wire the real one while tests inject a fake. Avoid package-level singletons for behavior; the only shared state is an explicitly passed config struct.

4. Flag parsing is abstracted behind reusable parser types. Non-trivial flags are modeled as small types in `flagparse` (`Parts`, `ServiceParams`, `Metadata`). They can self-register, handle merging and provide domain type construction utilities.