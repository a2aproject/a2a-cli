# Cookbook: Learn the a2a CLI by example

> New to the `a2a` CLI? Start with the [repo README](../README.md), then keep the [command reference](../internal/) handy.

👋 Welcome to the A2A CLI Cookbook.

This is a short, hands-on course. Each lesson is a small folder you can run on its own, and each one builds on the last. As you progress you will see how capable the `a2a` CLI is and get to know its richer features.

To get started you need only the `a2a` CLI installed. Every lesson starts its own example agent, so you can jump straight to any one of them.

## Lessons

### 1. Build a quick A2A server — [`01-exec-demo/`](01-exec-demo/)

The `a2a` CLI is a client for A2A servers. It sends messages, tracks task progress, and fetches results. So first you need a server to talk to.

Learn how to stand up a simple demo server. You will turn an ordinary script into a working A2A server with `--exec`, then send it a message and read the reply. No A2A-specific code required.

### 2. Discover and talk to an agent — [`02-card-and-send/`](02-card-and-send/)

Before you can use an agent, you need to know what it can do. Every A2A agent answers that with an **agent card**.

Learn the client side: read an agent's card, save it, set it once through config, and send a text message.

### 3. Configure the CLI — [`03-config/`](03-config/)

Learn the three ways to pass a setting: a CLI flag, a session environment variable, and a `.env` file. See how `a2a config show` tells you which one won, plus a table of every setting you can configure.

### 4. Messages and tasks — [`04-messages-and-tasks/`](04-messages-and-tasks/)

Go deeper: build messages from text, file, and data parts; stream a reply; and send async to get back a task id and watch its lifecycle. Includes a reference for the `task` commands.

## Extend the CLI

The `a2a` CLI supports plugin-style customization. See the [specification](../specification/SPEC.md) for details, and [`a2a-transport-echo/`](a2a-transport-echo/) for a working example.

## Learn more

Read the [a2a-cli specification](../specification/SPEC.md) for everything the tool offers.
