# How to contribute

We'd love to accept your patches and contributions to this project. This repository holds two things: the **A2A CLI specification** (`specification/SPEC.md`, `specification/COMPLIANCE.md`) and its **Go reference implementation** (`a2a`). Contributions to either are welcome.

## Ways to contribute

Code is only one way to help. The project grows through many kinds of contribution:

- **Request a feature.** Open a [feature request](https://github.com/a2aproject/a2a-cli/issues/new?template=feature-request.yml) to propose a new command, flag, or specification change.
- **Report a bug.** File a [bug report](https://github.com/a2aproject/a2a-cli/issues/new?template=bug-report.yml) with steps to reproduce, your CLI version, and your OS.
- **Contribute code.** Fix a bug or build a feature — see the workflow below.
- **Improve the docs.** Clarify the README, the specification, or these guidelines.
- **Share feedback.** Tell us how you use the tool, what works, and what's rough — on our [Discord server](https://discord.com/invite/9UpukKSpRN) or in an issue.
- **Help the community.** Answer questions, help reproduce and triage issues, and review pull requests.

## Before you start

- Read our [Code of Conduct](./CODE_OF_CONDUCT.md). By participating, you agree to follow it.
- For a quick chat or to say Hi, reach out on our [Discord server](https://discord.com/invite/9UpukKSpRN).
- For anything actionable, open an [issue](https://github.com/a2aproject/a2a-cli/issues) or [pull request](https://github.com/a2aproject/a2a-cli/pulls).

## Conventional Commits

Commit messages and pull request titles follow the [Conventional Commits](https://www.conventionalcommits.org/) specification. PR titles are lint-checked. Choose the type and scope based on what you changed:

- **CLI code** — `feat:` for new behavior, `fix:` for bug fixes (e.g., `feat: add subscribe command`).
- **SPEC or COMPLIANCE file** — `docs(spec):` for changes to `specification/SPEC.md` or `specification/COMPLIANCE.md`.
- **Other doc files** — `docs:` for all other documentation.

## Contribution process

### Code reviews

We use GitHub pull requests for reviews. See [GitHub Help](https://help.github.com/articles/about-pull-requests/) for how to use them.

### Workflow

1. **Fork** the official repository to your own account.
2. **Sync** your fork with the latest changes from `main`.
3. **Create a feature branch** and make your changes there.
4. **Commit** using a Conventional Commits message.
5. **Open a pull request** from your feature branch to the official repository's `main` branch.
6. **Resolve feedback** — work with reviewers to address comments.

Be patient — reviewing and merging a pull request can take time.

### Pull request tips

A few checks run on every pull request. Follow these tips to pass them:

- Add tests for every new feature.
- Open an issue first, then link it in the PR. For example: `Fixes #123` or `Closes #123`.
- Use Conventional Commits for commit messages and PR titles.
- For code changes, run these before you push:

  ```sh
  go mod tidy -diff
  go build ./...
  go test -race ./...
  golangci-lint run
  ```
