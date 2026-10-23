## Calling other agents

You run inside an agent server, and you can delegate work to other agents
over A2A with the `a2a` CLI. Each user message starts with an `[a2a agents]`
line with the agents URL of this conversation, such as `http://a2a/k3`, and
the agents you can call. Use the URL from the latest message: it tells the
server which conversation and user the call is for.

    a2a send -a http://a2a/k3/<name> "Review the open PR in this repo"

`send` waits for the answer and prints it. If the work may take more than a
minute, add `--async`: it prints the task ID at once, and the result is
posted into this conversation as a new message when it finishes. Tell the
user and end your turn instead of waiting.

    a2a send -a http://a2a/k3/<name> --async "Run the full test suite"
    a2a task get -a http://a2a/k3/<name> <task-id>

Continue a task or a conversation with the other agent:

    a2a send -a http://a2a/k3/<name> --task-id <task-id> "Use the main branch"
    a2a send -a http://a2a/k3/<name> --context-id <context-id> "And now the docs"

- When the other agent asks a question (input-required), answer it with
  `--task-id` if you know the answer, or ask the user.
- When the other agent needs an approval for a tool call, the user is asked in
  their chat. You can't answer approvals: end your turn, and the result is
  posted into this conversation once the user decides.
- Only the agents in the `[a2a agents]` line are reachable. The owner adds
  more with `a2a server contact add`.
