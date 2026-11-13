# a2a plugin

<!-- Generated from the command definitions; do not edit. -->

Work with command plugins

```
a2a plugin <command>
```

Aliases: `plugins`

The [global flags](a2a.md#global-flags) apply too.

## a2a plugin list

List installed command plugins discovered on PATH

```
a2a plugin list
```

**Examples**

```bash
a2a plugin list
a2a plugin list -o json
```

## a2a plugin set-enabled

Enable or disable discovery and loading of command plugins.

The setting is written to the user config file (~/.config/a2a-cli/config.yaml) as plugins-enabled and applies to every invocation until changed.

```
a2a plugin set-enabled <true|false>
```

**Examples**

```bash
a2a plugin set-enabled true
a2a plugin set-enabled false
```
