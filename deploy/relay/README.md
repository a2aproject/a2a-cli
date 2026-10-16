# Deploying an A2A relay (prototype)

These scripts run `a2a server relay serve`, the relay prototype described in
[docs/relay](../../docs/relay/README.md). After deploying, follow
[Pair Grok Bot with the relay](../../docs/relay/README.md#pair-grok-bot-with-the-relay)
to attach an agent.

| Script | What it does |
| --- | --- |
| [`local-demo.sh`](local-demo.sh) | Runs a relay and a stand-in webhook agent on localhost and walks through attach, pair, grant and a task. No cloud account needed. |
| [`cloudrun/deploy.sh`](cloudrun/deploy.sh) | Deploys the relay to Google Cloud Run. |
| [`cloudrun/cleanup.sh`](cloudrun/cleanup.sh) | Deletes everything `deploy.sh` created. |

## Cloud Run

Prerequisites: `gcloud` logged in to a project where you can enable APIs and
create service accounts, buckets and Cloud Run services, plus `docker`, `go`
and `openssl`. The relay must accept unauthenticated traffic from the
internet, because callers and the agent's vendor reach it directly. The
project's org policies have to allow public ingress and `allUsers` as a Cloud
Run invoker. Projects that forbid either can't host the relay.

```sh
PROJECT=<project> deploy/relay/cloudrun/deploy.sh
```

The script:

1. Enables the Cloud Run, Artifact Registry, IAM and Cloud Storage APIs.
2. Builds the `a2a` binary for linux/amd64 locally, wraps it in a distroless
   image, and pushes the image to Artifact Registry. No Cloud Build is
   involved.
3. Creates a runtime service account and a Cloud Storage bucket for the relay
   state (attached nodes, tokens and grants), mounted at `/state`.
4. Deploys a single always-on instance (`--min-instances 1 --max-instances 1
   --no-cpu-throttling`), because tasks are held in memory.
5. Prints the relay URL and, on the first deploy, a generated admin token.
   Keep the token. Later deploys keep the existing token unless you set
   `A2A_RELAY_ADMIN_TOKEN`.

Options, all as environment variables:

| Variable | Default |
| --- | --- |
| `PROJECT` | The default `gcloud` project |
| `REGION` | `us-central1` |
| `SERVICE` | `a2a-relay` (also the service account name) |
| `BUCKET` | `$PROJECT-a2a-relay-state` |
| `REPO` | `a2a-relay` (Artifact Registry repository) |
| `A2A_RELAY_ADMIN_TOKEN` | Generated on the first deploy |

Check the deployment:

```sh
curl -s https://<relay>/healthz        # {"status": "ok"}
curl -s https://<relay>/skill.md       # the a2a-relay skill, with the relay URL filled in
```

Then export the values the script printed and attach an agent:

```sh
export A2A_RELAY_URL=https://<relay> A2A_RELAY_ADMIN_TOKEN=<admin token>
a2a server relay attach <webhook-url> --name <agent> --webhook-header "Authorization: Bearer <secret>"
```

To update the relay after code changes, run the script again.

### Shutting it down

```sh
PROJECT=<project> deploy/relay/cloudrun/cleanup.sh          # asks before deleting; --yes skips the prompt
```

This deletes the Cloud Run service, the Artifact Registry repository with its
images, the state bucket and the service account. Pass the same `REGION`,
`SERVICE`, `BUCKET` and `REPO` you deployed with. Deleting the bucket removes
every attached node, token and grant, so agents have to be attached and paired
again after a redeploy. Set `KEEP_BUCKET=1` to keep it. Resources that are
already gone are skipped, and the APIs that `deploy.sh` enabled stay enabled.

## Other hosts

The relay is a single binary that runs anywhere with a public HTTPS address:

```sh
A2A_RELAY_ADMIN_TOKEN=<token> a2a server relay serve \
  --addr :8090 --public-url https://relay.example.com --state-dir /var/lib/a2a-relay
```

- Put it behind a TLS-terminating proxy. Without `--public-url`, the relay
  derives its URL from the `Host` and `X-Forwarded-Proto` headers.
- Run exactly one instance, and keep it running. A restart loses in-flight
  tasks but keeps attached nodes in `--state-dir`.
- Agents download the CLI from `/download/a2a`, which serves the relay's own
  binary. Build it for the platform the agents run on (usually linux/amd64).
