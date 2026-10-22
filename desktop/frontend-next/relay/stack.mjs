import http from "node:http";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawn } from "node:child_process";

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

export async function until(what, probe, ms = 20_000) {
  const end = Date.now() + ms;
  for (;;) {
    const value = await probe();
    if (value) return value;
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await wait(100);
  }
}

function firstLine(child, what) {
  return new Promise((resolve, reject) => {
    let buffer = "";
    child.stdout.on("data", (chunk) => {
      buffer += chunk;
      const at = buffer.indexOf("\n");
      if (at >= 0) resolve(buffer.slice(0, at));
    });
    child.once("exit", (code) => reject(new Error(`${what} exited (${code}) before it was ready`)));
  });
}

function scriptedProvider(words, gapMs) {
  const requests = [];
  const server = http.createServer((req, res) => {
    let body = "";
    req.on("data", (chunk) => { body += chunk; });
    req.on("end", () => {
      if (!req.url.includes("chat/completions")) {
        res.writeHead(404);
        res.end("{}");
        return;
      }
      requests.push({ at: Date.now(), body });
      res.writeHead(200, { "content-type": "text/event-stream" });
      const chunk = (delta) =>
        `data: ${JSON.stringify({ id: "x", object: "chat.completion.chunk", model: "fake-m", choices: [{ index: 0, delta }] })}\n\n`;
      let n = 0;
      const timer = setInterval(() => {
        if (n < words) {
          res.write(chunk({ content: `word${n++} ` }));
          return;
        }
        clearInterval(timer);
        res.write(`data: ${JSON.stringify({ id: "x", object: "chat.completion.chunk", model: "fake-m", choices: [{ index: 0, delta: {}, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 12, total_tokens: 22 } })}\n\n`);
        res.write("data: [DONE]\n\n");
        res.end();
      }, gapMs);
      res.on("close", () => clearInterval(timer));
    });
  });
  return new Promise((resolve) => server.listen(0, "127.0.0.1", () => resolve({ server, requests, port: server.address().port })));
}

function pageServer(dist) {
  const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".woff2": "font/woff2", ".json": "application/json", ".webmanifest": "application/manifest+json", ".png": "image/png" };
  const server = http.createServer((req, res) => {
    const target = path.join(dist, decodeURIComponent(new URL(req.url, "http://x").pathname));
    const file = target.startsWith(dist) && fs.existsSync(target) && fs.statSync(target).isFile() ? target : path.join(dist, "index.html");
    res.writeHead(200, { "content-type": types[path.extname(file)] ?? "application/octet-stream" });
    fs.createReadStream(file).pipe(res);
  });
  return new Promise((resolve) => server.listen(0, "127.0.0.1", () => resolve({ server, origin: `http://127.0.0.1:${server.address().port}` })));
}

/**
 * Starts everything between a hosted page and a Studio except the page's browser:
 * the real studio-host binary on a fresh data home, a stand-in accounts API and
 * relay, a scripted model provider, and a static server for the built page.
 */
export async function startStack({ hostBin, fakeBin, dist, servePage = false, words = 12, gapMs = 120 }) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "relay-stack-"));
  const home = path.join(root, "home");
  const workspace = path.join(root, "workspace");
  fs.mkdirSync(home, { recursive: true });
  fs.mkdirSync(workspace, { recursive: true });
  const provider = await scriptedProvider(words, gapMs);
  fs.writeFileSync(path.join(home, "config.toml"), [
    'language = "en"',
    'default_model = "fake"',
    `providers = [{ name = "fake", kind = "openai", base_url = "http://127.0.0.1:${provider.port}/v1", model = "fake-m", api_key_env = "FAKE_KEY" }]`,
    "",
  ].join("\n"));
  fs.writeFileSync(path.join(home, ".env"), "FAKE_KEY=fixture\n");

  const fake = spawn(fakeBin, [], { stdio: ["ignore", "pipe", "inherit"] });
  const urls = JSON.parse(await firstLine(fake, "relayfake"));
  const ctl = async (route, method = "GET") => (await fetch(`${urls.accounts}/ctl/${route}`, { method })).json();

  const host = spawn(hostBin, servePage ? ["-page", dist] : [], {
    stdio: ["pipe", "pipe", "inherit"],
    env: {
      ...process.env, HOME: root, USERPROFILE: root, REASONIX_HOME: home, REASONIX_STATE_HOME: home,
      REASONIX_ACCOUNTS_URL: urls.accounts, REASONIX_REMOTE_URL: urls.relay, REASONIX_ACCOUNT_TOKEN: "fixture-token",
      FAKE_KEY: "fixture",
    },
  });
  const handshake = JSON.parse(await firstLine(host, "studio-host"));
  const state = await until("the Studio to reach the relay", async () => {
    const s = await ctl("state");
    return s.deviceId && s.deviceOnline ? s : null;
  });
  const page = await pageServer(dist);

  const api = async (route, method = "GET", body) => {
    const response = await fetch(handshake.origin + route, {
      method,
      headers: { cookie: `reasonix_token=${handshake.token}`, origin: handshake.origin, "content-type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    return { status: response.status, json: await response.json().catch(() => null) };
  };

  return {
    urls, deviceId: state.deviceId, pageOrigin: page.origin, hostOrigin: handshake.origin, hostToken: handshake.token, workspace, provider, ctl, api,
    pageUrl: () => `${page.origin}/?device=${state.deviceId}`,
    async stop() {
      const gone = new Promise((resolve) => host.once("exit", resolve));
      host.stdin.end();
      host.kill();
      await gone;
      fake.kill();
      page.server.close();
      provider.server.close();
      fs.rmSync(root, { recursive: true, force: true });
    },
  };
}

/**
 * Points a browser context's two Reasonix services at the stand-ins. The page is
 * the production build, so it still dials id.reasonix.io and remote.reasonix.io;
 * the requests and the socket are answered here. The socket is a plain pipe:
 * the page and the Studio speak their own encrypted protocol across it.
 */
export async function routeServices(context, stack) {
  await context.route("https://id.reasonix.io/**", async (route) => {
    const request = route.request();
    const cors = {
      "access-control-allow-origin": request.headers().origin ?? "*",
      "access-control-allow-credentials": "true",
      "access-control-allow-headers": "content-type",
    };
    if (request.method() === "OPTIONS") {
      await route.fulfill({ status: 204, headers: cors });
      return;
    }
    const answer = await fetch(stack.urls.accounts + new URL(request.url()).pathname, {
      method: request.method(),
      headers: { "content-type": "application/json" },
      body: request.postData() ?? undefined,
    });
    await route.fulfill({ status: answer.status, headers: { ...cors, "content-type": "application/json" }, body: await answer.text() });
  });
  await context.routeWebSocket(/remote\.reasonix\.io/, (page) => {
    const upstream = new WebSocket(`${stack.urls.relay}/v1/sessions/connect?fixture=1`, "reasonix.remote.v1");
    const queued = [];
    upstream.onopen = () => queued.splice(0).forEach((message) => upstream.send(message));
    upstream.onmessage = (event) => page.send(String(event.data));
    upstream.onclose = (event) => page.close({ code: event.code >= 1000 && event.code < 5000 ? event.code : 1011, reason: event.reason });
    page.onMessage((message) => (upstream.readyState === 1 ? upstream.send(message) : queued.push(message)));
    page.onClose(() => upstream.close());
  });
}
