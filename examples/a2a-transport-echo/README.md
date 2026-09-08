# a2a-transport-echo

A reference [A2A CLI transport plugin](../../docs/transport-plugins.md) built with
the [`clitransport` devkit](../../devkit/clitransport).

It implements a custom `a2aclient.Transport` that does not talk to any real
upstream — it simply echoes the caller's message back as a completed task. It
exists to demonstrate the plugin contract end to end.

## Build and install

```console
$ go build -o a2a-transport-echo .
$ mv a2a-transport-echo ~/bin/    # anywhere on your PATH
```

## Try it

```console
$ a2a transport list
NAME  VERSION  PROTOCOL  DESCRIPTION                                           PATH
echo  1.0.0    1.0       Echoes the caller's message back as a completed task  …/a2a-transport-echo

$ a2a send --transport echo --endpoint echo://demo "hello there"
Task:     …
Status:   completed
Artifacts:
  […] hello there

$ a2a send --transport echo --endpoint echo://demo --stream -o json "stream me"
{ "task": { … "state": "TASK_STATE_SUBMITTED" } }
{ "statusUpdate": { … "state": "TASK_STATE_WORKING" } }
{ "artifactUpdate": { … "text": "stream me" … } }
{ "statusUpdate": { … "state": "TASK_STATE_COMPLETED" } }
```

## What to look at

* [`main.go`](main.go) — wires the plugin with `clitransport.Main`.
* [`echo.go`](echo.go) — the custom `a2aclient.Transport` implementation.

Everything else — subcommand parsing, the loopback server, the handshake, the
per-launch token and graceful shutdown — is provided by the devkit.
