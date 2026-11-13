# Lesson 2: Configuration

In lesson 1 you saved the agent's address in a `.env` file. That is one of several places `a2a` reads settings from. In this lesson you set the same setting in each of them, and use `a2a config show` to see which one won and why.

![Setting a value as a flag, a variable, in .env and in config.yaml, and checking each with config show](demo.gif)

## What you'll learn

- The places a setting can come from, and their order
- How flag names map to `A2ACLI_*` variables and config file keys
- How `a2a config show` reports each value and its source
- Which settings a project's `.env` can't set, and why

## Prerequisites

The agent from [lesson 1](../01-first-message/README.md), still running. If you stopped it, start it again in the background and update `.env`:

```bash
cd ~/a2a-lessons/agent && a2a-serve start --detach
cd ~/a2a-lessons && echo "A2ACLI_AGENT_CARD=$(a2a-serve status --dir agent --url=local)" > .env
```

Run everything below in `~/a2a-lessons`.

## Step 1: See the current configuration

```bash
a2a config show
```

```text
SETTING            VALUE                   SOURCE
a2a-version        (unset)                 default
agent-card         http://127.0.0.1:43229  local-file
auth               <redacted>              default
ca-cert            (unset)                 default
catalog            (unset)                 default
credential-helper  a2a-serve credential    user-file
endpoint           (unset)                 default
insecure           false                   default
output             text                    default
svc-param          (unset)                 default
tenant             (unset)                 default
timeout            30s                     default
transport          (unset)                 default
unix-socket        (unset)                 default
verbose            false                   default
```

Every global flag is listed with its value and where the value came from. Two are already set: `agent-card` from your `.env` (`local-file`), and `credential-helper` from `~/.config/a2a-cli/config.yaml` (`user-file`), where `a2a-serve doctor --fix` put it. Lesson 5 explains the helper.

## Step 2: One setting, four places

Take `--timeout`, which bounds how long `a2a` waits. Its default is `30s`.

**A config file for all your commands.** `~/.config/a2a-cli/config.yaml` uses the flag names as keys:

```bash
echo "timeout: 45s" >> ~/.config/a2a-cli/config.yaml
a2a config show | grep timeout
```

```text
timeout            45s                     user-file
```

**A project file.** In a `.env` file, the key is the flag name in upper snake case with an `A2ACLI_` prefix:

```bash
echo "A2ACLI_TIMEOUT=1m" >> .env
a2a config show | grep timeout
```

```text
timeout            1m                      local-file
```

**An environment variable** for the rest of this shell session:

```bash
export A2ACLI_TIMEOUT=2m
a2a config show | grep timeout
```

```text
timeout            2m                      env
```

**A flag** for one command:

```bash
a2a config show --timeout 3m | grep timeout
```

```text
timeout            3m0s                    flag
```

Each one beat the one before. A flag's value prints the way `a2a` parsed it, here `3m0s`; values from files and variables print as written. The full order, first match wins:

1. a flag
2. an `A2ACLI_*` environment variable
3. the local file: the one `--config` names, or the nearest `.env` at or above the working directory
4. `~/.config/a2a-cli/config.yaml`
5. `~/.config/a2a-cli/.env`
6. the built-in default

Clean up before moving on:

```bash
unset A2ACLI_TIMEOUT
sed -i.bak '/^timeout:/d' ~/.config/a2a-cli/config.yaml
sed -i.bak '/^A2ACLI_TIMEOUT=/d' .env
rm ~/.config/a2a-cli/config.yaml.bak .env.bak
```

## Step 3: Pick a file with `--config`

`--config` loads a file in place of `.env`. It can be `.env`, YAML or JSON, which suits settings you switch between, such as one file per agent:

```bash
cat > verbose.yaml <<'EOF'
verbose: true
output: json
EOF
a2a --config verbose.yaml config show | jq -r '.[] | select(.source == "local-file") | "\(.name)=\(.value) from \(.path)"'
```

```text
output=json from verbose.yaml
verbose=true from verbose.yaml
```

`verbose.yaml` sets `output: json`, so `config show` printed JSON, which includes the path each value came from. The agent card is unset now, because `verbose.yaml` replaced `.env`.

## Step 4: Settings a project can't set

A `.env` file can come with a repository you cloned. Three settings would let it take over your requests: `--credential-helper` runs a program, `--unix-socket` redirects every connection, and `--ca-cert` decides which certificates to trust. `a2a` ignores them in a `.env` it found by walking up, and says so:

```bash
echo "A2ACLI_CA_CERT=/tmp/evil.pem" >> .env
a2a config show | grep ca-cert
sed -i.bak '/^A2ACLI_CA_CERT=/d' .env && rm .env.bak
```

```text
warning: ignoring ca-cert from /home/you/a2a-lessons/.env; pass --config /home/you/a2a-lessons/.env to use it
ca-cert            (unset)                 default
```

Set them as flags, variables, in `config.yaml`, or in a file you pass with `--config`.

## Recap

- Every flag has an `A2ACLI_*` variable and a config file key.
- Flags beat variables, variables beat files, and the project file beats your user files.
- `a2a config show` tells you each value and its source; `-o json` adds the file path.
- `--stream`, `--config`, `--help` and `--version` only work as flags.

## Next

[Lesson 3](../03-messages-and-tasks/README.md) sends richer messages and follows tasks that take a while.
