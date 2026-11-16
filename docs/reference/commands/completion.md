# a2a completion

<!-- Generated from the command definitions; do not edit. -->

Generate the autocompletion script for a2a for the specified shell.
See each sub-command's help for details on how to use the generated script.

```
a2a completion <command>
```

The [global flags](a2a.md#global-flags) apply too.

## a2a completion bash

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source &lt;(a2a completion bash)

To load completions for every new session, execute once:

#### Linux:

	a2a completion bash &gt; /etc/bash_completion.d/a2a

#### macOS:

	a2a completion bash &gt; $(brew --prefix)/etc/bash_completion.d/a2a

You will need to start a new shell for this setup to take effect.

```
a2a completion bash
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--no-descriptions` |  | disable completion descriptions |

## a2a completion fish

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	a2a completion fish | source

To load completions for every new session, execute once:

	a2a completion fish &gt; ~/.config/fish/completions/a2a.fish

You will need to start a new shell for this setup to take effect.

```
a2a completion fish [flags]
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--no-descriptions` |  | disable completion descriptions |

## a2a completion powershell

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	a2a completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.

```
a2a completion powershell [flags]
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--no-descriptions` |  | disable completion descriptions |

## a2a completion zsh

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" &gt;&gt; ~/.zshrc

To load completions in your current shell session:

	source &lt;(a2a completion zsh)

To load completions for every new session, execute once:

#### Linux:

	a2a completion zsh &gt; "${fpath[1]}/_a2a"

#### macOS:

	a2a completion zsh &gt; $(brew --prefix)/share/zsh/site-functions/_a2a

You will need to start a new shell for this setup to take effect.

```
a2a completion zsh [flags]
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--no-descriptions` |  | disable completion descriptions |
