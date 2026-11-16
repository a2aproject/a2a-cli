# How to contribute

We'd love to accept your patches and contributions to this project. This repository holds the `a2a` CLI, its documentation site, and its lessons and examples. Contributions to any of them are welcome.

## Ways to contribute

Code is only one way to help. The project grows through many kinds of contribution:

- **Request a feature.** Open a [feature request](https://github.com/a2aproject/a2a-cli/issues/new?template=feature-request.yml) to propose a new command or flag, or a change to an existing one.
- **Report a bug.** File a [bug report](https://github.com/a2aproject/a2a-cli/issues/new?template=bug-report.yml) with steps to reproduce, your CLI version, and your OS.
- **Contribute code.** Fix a bug or build a feature — see the workflow below.
- **Improve the docs.** Clarify the [documentation](https://a2aproject.github.io/a2a-cli/), the lessons and examples, or these guidelines. [Documentation](#documentation) below explains how they are built.
- **Share feedback.** Tell us how you use the tool, what works, and what's rough — on our [Discord server](https://discord.com/invite/9UpukKSpRN) or in an issue.
- **Help the community.** Answer questions, help reproduce and triage issues, and review pull requests.

## Before you start

- Read our [Code of Conduct](./CODE_OF_CONDUCT.md). By participating, you agree to follow it.
- For a quick chat or to say Hi, reach out on our [Discord server](https://discord.com/invite/9UpukKSpRN).
- For anything actionable, open an [issue](https://github.com/a2aproject/a2a-cli/issues) or [pull request](https://github.com/a2aproject/a2a-cli/pulls).

## Conventional Commits

Commit messages and pull request titles follow the [Conventional Commits](https://www.conventionalcommits.org/) specification. PR titles are lint-checked. Choose the type and scope based on what you changed:

- **CLI code** — `feat:` for new behavior, `fix:` for bug fixes (e.g., `feat: add subscribe command`).
- **Documentation** — `docs:` for the docs, lessons and examples.

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

## Documentation

The documentation site is built with [MkDocs Material](https://squidfunk.github.io/mkdocs-material/) from `docs/`, `lessons/` and `examples/`, and published to GitHub Pages from `main`. The markdown renders on GitHub as well, so link between pages with relative links to the `.md` files.

- `docs/guide/` is the User Guide: one short page per task.
- `docs/advanced/` explains how the CLI works inside.
- `docs/reference/commands/` is generated from the command definitions. After changing a command or flag, run `go run ./internal/tools/gendocs`; a test fails while the pages are out of date.
- `lessons/` is the course, and `examples/` holds recipes. Each has a `demo.gif` recorded from a scenario in `tools/demo/scenarios/`; [tools/demo/README.md](tools/demo/README.md) explains how to record one. When you change the commands in a lesson or example, change its scenario too: CI plays every scenario.

Build the site locally:

```sh
pip install -r site/requirements.txt
site/build.sh          # build into site/.out/site
site/build.sh serve    # serve with live reload
```
