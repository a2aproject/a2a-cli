# a2a config

<!-- Generated from the command definitions; do not edit. -->

Inspect configuration

```
a2a config <command>
```

The [global flags](a2a.md#global-flags) apply too.

## a2a config show

Show the effective configuration and where each value resolved from.

Change settings by exporting the matching A2ACLI_\* environment variables or adding them to a .env file.

```
a2a config show
```

**Examples**

```bash
a2a config show
a2a config show -o json
a2a --config ./prod.yaml config show
```
