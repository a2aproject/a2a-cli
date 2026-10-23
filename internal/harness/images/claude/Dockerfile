# Harness image for `a2a server claude`. The server runs it as the host
# user with the workspace mounted at its host path and HOME=/home/agent.
# The harness calls other agents with the a2a CLI; on Linux hosts the server
# mounts its own binary over this one. The server builds the image with
# A2A_CLI_VERSION set to its own commit so both sides speak the same protocol.
FROM golang:1.26-bookworm AS a2a
ARG A2A_CLI_VERSION=latest
RUN CGO_ENABLED=0 GOBIN=/out go install -trimpath github.com/a2aproject/a2a-cli@${A2A_CLI_VERSION}

FROM node:22-bookworm-slim
COPY --from=a2a /out/a2a-cli /usr/local/bin/a2a
RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates curl ripgrep openssh-client python3 \
 && rm -rf /var/lib/apt/lists/*
RUN npm install -g @zed-industries/claude-agent-acp@latest @anthropic-ai/claude-code@latest && npm cache clean --force
RUN mkdir -p /home/agent && chmod 1777 /home/agent
ENV HOME=/home/agent
CMD ["claude-agent-acp"]
