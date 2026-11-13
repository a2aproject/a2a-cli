# Push Notifications

An agent that supports push notifications posts task updates to a webhook URL you give it, so a service can learn that a task finished without polling. `a2a task push-config` (or `a2a task push`) manages these webhooks.

| Command | What it does |
|---|---|
| `a2a task push-config create <task-id> --url <url>` | Registers a webhook for a task. |
| `a2a task push-config list <task-id>` | Lists a task's webhooks. |
| `a2a task push-config get <task-id> <config-id>` | Prints one. |
| `a2a task push-config delete <task-id> <config-id>` | Removes one. |

```bash
a2a task push-config create -a localhost:8080 01a1d530-53cf-74b7-a36c-af9ea1df605e \
  --url https://hooks.example.com/a2a \
  --token "$(openssl rand -hex 16)" \
  --auth-scheme Bearer --auth-credentials "$HOOK_TOKEN"
```

| Flag | Sets |
|---|---|
| `--url` | The webhook the agent posts to. Required. |
| `--id` | Your ID for this configuration, to register more than one per task. |
| `--token` | A value the agent sends back with each call, so the receiver can check it. |
| `--auth-scheme`, `--auth-credentials` | How the agent authenticates to the webhook, such as `Bearer` and a token. |

Text output hides the credentials; `-o json` prints the configuration as the agent returned it.

Agents that don't support push notifications answer `push notification not supported`. The card's capabilities say whether an agent does: `a2a card get <ref> -o json | jq .capabilities.pushNotifications`.
