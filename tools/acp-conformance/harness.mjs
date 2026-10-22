import { spawn } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Readable, Writable } from "node:stream";
import * as acp from "@agentclientprotocol/sdk";
import Ajv2020 from "ajv/dist/2020.js";
import { fakeModel } from "./fakemodel.mjs";

const require = createRequire(import.meta.url);
const schema = JSON.parse(readFileSync(require.resolve("@agentclientprotocol/sdk/schema/schema.json"), "utf8"));
const ajv = new Ajv2020({ strict: false, allErrors: false });
const range = (min, max) => ({ type: "number", validate: (n) => Number.isInteger(n) && n >= min && n <= max });
for (const [name, bounds] of Object.entries({
  int32: [-(2 ** 31), 2 ** 31 - 1], uint16: [0, 65535], uint32: [0, 2 ** 32 - 1],
  int64: [Number.MIN_SAFE_INTEGER, Number.MAX_SAFE_INTEGER], uint64: [0, Number.MAX_SAFE_INTEGER],
})) ajv.addFormat(name, range(...bounds));
ajv.addFormat("double", { type: "number", validate: Number.isFinite });
ajv.addFormat("uri", { type: "string", validate: (u) => URL.canParse(u) });
const check = new Map();
function validator(def) {
  if (!check.has(def)) check.set(def, ajv.compile({ $ref: `#/$defs/${def}`, $defs: schema.$defs }));
  return check.get(def);
}

// Agent responses by the request that produced them: the shipped schema names
// one definition per method, so each answer is held to exactly its own.
const RESPONSES = {
  initialize: "InitializeResponse",
  "session/new": "NewSessionResponse",
  "session/load": "LoadSessionResponse",
  "session/list": "ListSessionsResponse",
  "session/resume": "ResumeSessionResponse",
  "session/close": "CloseSessionResponse",
  "session/delete": "DeleteSessionResponse",
  "session/set_mode": "SetSessionModeResponse",
  "session/set_config_option": "SetSessionConfigOptionResponse",
  "session/prompt": "PromptResponse",
};

export const REASONIX_BIN = process.env.REASONIX_BIN;
if (!REASONIX_BIN) throw new Error("set REASONIX_BIN to a built reasonix binary");

// Starts `reasonix acp` against a scripted model and records every line it
// writes, so a test can hold the raw wire to the official schema as well as to
// the SDK's own parsing.
export async function startAgent({ script = (n) => ({ text: `answer ${n}` }), config = "" } = {}) {
  const home = mkdtempSync(join(tmpdir(), "acp-home-"));
  const work = realpathSync(mkdtempSync(join(tmpdir(), "acp-work-")));
  const model = await fakeModel(script);
  writeFileSync(join(home, "config.toml"), `default_model = "fake/m"\n${config}\n[[providers]]\nname = "fake"\nkind = "openai"\nbase_url = "${model.url}"\nmodels = ["m"]\ndefault = "m"\napi_key_env = "FAKE_KEY"\n`);
  const child = spawn(REASONIX_BIN, ["acp"], {
    cwd: work,
    env: { ...process.env, REASONIX_HOME: home, FAKE_KEY: "k", REASONIX_CREDENTIALS_STORE: "file" },
    stdio: ["pipe", "pipe", "pipe"],
  });
  let stderr = "";
  child.stderr.on("data", (d) => (stderr += d));
  const wire = { out: [], inn: [] };
  let pending = "";
  child.stdout.on("data", (chunk) => {
    pending += chunk;
    let nl;
    while ((nl = pending.indexOf("\n")) >= 0) {
      const line = pending.slice(0, nl).trim();
      pending = pending.slice(nl + 1);
      if (line) wire.out.push(JSON.parse(line));
    }
  });
  const updates = [];
  const permissions = [];
  let answerPermission = (request) => ({ outcome: { outcome: "selected", optionId: request.options[0].optionId } });
  const client = {
    async requestPermission(request) {
      permissions.push(request);
      return answerPermission(request);
    },
    async sessionUpdate(notification) {
      updates.push(notification);
    },
  };
  const write = child.stdin.write.bind(child.stdin);
  child.stdin.write = (chunk, ...rest) => {
    for (const line of Buffer.from(chunk).toString("utf8").split("\n")) if (line.trim()) wire.inn.push(JSON.parse(line));
    return write(chunk, ...rest);
  };
  const stream = acp.ndJsonStream(Writable.toWeb(child.stdin), Readable.toWeb(child.stdout));
  const conn = new acp.ClientSideConnection(() => client, stream);
  return {
    conn, wire, updates, permissions, model, work, home,
    // A request the SDK has no method for, answered by whatever the agent writes back.
    async rpc(method, params, id = `raw-${wire.inn.length}`) {
      child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", id, method, params })}\n`);
      for (let waited = 0; waited < 5000; waited += 20) {
        const found = wire.out.find((m) => m.id === id && ("result" in m || "error" in m));
        if (found) return found;
        await new Promise((r) => setTimeout(r, 20));
      }
      throw new Error(`no answer to ${method}`);
    },
    notify(method, params) {
      child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", method, params })}\n`);
    },
    onPermission(fn) { answerPermission = fn; },
    stderr: () => stderr,
    async stop() {
      child.kill();
      await model.close();
      rmSync(home, { recursive: true, force: true });
      rmSync(work, { recursive: true, force: true });
    },
  };
}

// Every message the agent wrote must satisfy the official schema. Returns the
// problems, so a failing test names the message and the reason.
export function wireProblems(wire) {
  const problems = [];
  const methodOf = new Map(wire.inn.filter((m) => m.method && m.id !== undefined).map((m) => [m.id, m.method]));
  for (const message of wire.out) {
    if (message.method === "session/update") {
      if (!validator("SessionNotification")(message.params)) problems.push(`session/update: ${ajv.errorsText(validator("SessionNotification").errors)} in ${JSON.stringify(message.params).slice(0, 200)}`);
    } else if (message.method === "session/request_permission") {
      if (!validator("RequestPermissionRequest")(message.params)) problems.push(`session/request_permission: ${ajv.errorsText(validator("RequestPermissionRequest").errors)}`);
    } else if ("result" in message && methodOf.has(message.id) && RESPONSES[methodOf.get(message.id)]) {
      const def = RESPONSES[methodOf.get(message.id)];
      if (!validator(def)(message.result)) problems.push(`${methodOf.get(message.id)}: ${ajv.errorsText(validator(def).errors)} in ${JSON.stringify(message.result).slice(0, 200)}`);
    } else if ("error" in message) {
      if (typeof message.error?.code !== "number" || typeof message.error?.message !== "string") problems.push(`malformed error: ${JSON.stringify(message.error)}`);
    }
  }
  return problems;
}
