// Copyright 2026 The A2A Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Runs `a2a hook` for opencode, which has no command hooks: it translates
// opencode's plugin events into the hook events `a2a hook` reads, and adds the
// context it prints to the conversation.

import { spawn } from "node:child_process"
import { randomBytes } from "node:crypto"

const TIMEOUT_MS = 5000
// How long a web tool call waits for the background probe of the origins it
// mentioned, so the agent hears about an agent found there in the tool result.
// `a2a hook` waits only when it started a probe.
const PROBE_WAIT = "1500ms"
// opencode's tool IDs mapped to the names `a2a hook` scans by default.
const TOOL_NAMES = { webfetch: "WebFetch", websearch: "WebSearch" }

function runHook(event, args = []) {
  return new Promise((resolve) => {
    let out = ""
    let child
    try {
      child = spawn("a2a", ["hook", ...args], { stdio: ["pipe", "pipe", "inherit"] })
    } catch {
      resolve("")
      return
    }
    const timer = setTimeout(() => child.kill(), TIMEOUT_MS)
    child.on("error", () => {
      clearTimeout(timer)
      resolve("")
    })
    // An a2a that exits without reading the event, such as one that predates
    // `a2a hook`, closes the pipe; the write error must not crash opencode.
    child.stdin.on("error", () => {})
    child.stdout.on("data", (chunk) => (out += chunk))
    child.on("close", () => {
      clearTimeout(timer)
      try {
        resolve(JSON.parse(out).hookSpecificOutput?.additionalContext ?? "")
      } catch {
        resolve("")
      }
    })
    child.stdin.end(JSON.stringify(event))
  })
}

// partID makes an ID in opencode's ascending format, which orders the part
// after the ones already in the message.
let lastID = 0n
function partID() {
  let now = BigInt(Date.now()) * 0x1000n
  if (now <= lastID) now = lastID + 1n
  lastID = now
  const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
  const random = Array.from(randomBytes(14), (b) => chars[b % 62]).join("")
  return "prt_" + (now & 0xffffffffffffn).toString(16).padStart(12, "0") + random
}

export const A2AHook = async ({ client, directory }) => {
  const started = new Set()
  // Context restored after a compaction, which has no message to attach it to.
  const compacted = new Map()

  const hook = (sessionID, fields, args) => runHook({ session_id: sessionID, cwd: directory, ...fields }, args)

  // A session this process hasn't seen is new when it has no messages yet, and
  // resumed otherwise, as with `opencode run --continue`.
  const startSource = async (sessionID) => {
    try {
      const res = await client.session.messages({ path: { id: sessionID } })
      return res.data?.length ? "resume" : "startup"
    } catch {
      return "startup"
    }
  }

  return {
    event: async ({ event }) => {
      if (event.type !== "session.compacted") return
      const sessionID = event.properties.sessionID
      const context = await hook(sessionID, { hook_event_name: "SessionStart", source: "compact" })
      if (context) compacted.set(sessionID, context)
    },

    "chat.message": async ({ sessionID }, output) => {
      const contexts = []
      if (!started.has(sessionID)) {
        started.add(sessionID)
        const source = await startSource(sessionID)
        contexts.push(await hook(sessionID, { hook_event_name: "SessionStart", source }))
      }
      const prompt = output.parts
        .filter((p) => p.type === "text" && !p.synthetic)
        .map((p) => p.text)
        .join("\n")
      contexts.push(await hook(sessionID, { hook_event_name: "UserPromptSubmit", prompt }))
      for (const text of contexts.filter(Boolean)) {
        output.parts.push({ id: partID(), sessionID, messageID: output.message.id, type: "text", text, synthetic: true })
      }
    },

    "tool.execute.after": async ({ tool, sessionID, args }, output) => {
      const toolName = TOOL_NAMES[tool]
      if (!toolName) return
      const event = {
        hook_event_name: "PostToolUse",
        tool_name: toolName,
        tool_input: args,
        tool_response: output.output,
      }
      const context = await hook(sessionID, event, ["--discovery-wait", PROBE_WAIT])
      if (context) output.output += "\n\n<system-reminder>\n" + context + "\n</system-reminder>"
    },

    "experimental.chat.system.transform": async ({ sessionID }, output) => {
      const context = compacted.get(sessionID)
      if (context) output.system.push(context)
    },
  }
}
