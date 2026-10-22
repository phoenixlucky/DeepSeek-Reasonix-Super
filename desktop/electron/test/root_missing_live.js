"use strict";
// With the project folder gone, a tool call is refused with the typed cause and the
// window says so above the composer, with a way to choose the folder again.
const os = require("node:os");
const path = require("node:path");
const fs = require("node:fs");
const { execFileSync } = require("node:child_process");
const { app } = require("electron");
const { checker, listen, scriptedModel, seedHome, settledWindow, until, wait } = require("./livekit");

const { check, failures } = checker();

async function main() {
  let calls = 0;
  const { handler, requests } = scriptedModel(() => {
    calls++;
    if (calls === 1) return { name: "grep", arguments: { pattern: "hello" } };
    return null;
  });
  const model = await listen(handler);
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "rx-root-live-"));
  const workspace = fs.mkdtempSync(path.join(os.tmpdir(), "rx-root-ws-"));
  process.env.REASONIX_HOME = home;
  seedHome(home, model.url, workspace);
  const { current } = require("../src/main.js");

  try {
    const win = await settledWindow();
    const { client } = current();
    const runtimes = await until("a pane", async () => {
      const list = await client.json("GET", "/runtimes");
      return Array.isArray(list) && list.length ? list : null;
    });
    process.chdir(os.tmpdir());
    const gone = `${workspace}-moved`;
    fs.renameSync(workspace, gone);
    await client.request("POST", `${runtimes[0].base}/submit`, { input: "look for hello" });
    await until("the scripted turn", async () => requests.length >= 2, 60000);
    const result = String((requests[1].messages || []).filter((m) => m.role === "tool").at(-1)?.content || "");
    check("the model was told the typed cause, not a bare system error", result.startsWith("root missing: the project folder ") && result.includes("not a permission or argument problem"), result.slice(0, 200));
    const js = (src) => win.webContents.executeJavaScript(src);
    const banner = await until("the banner above the composer", async () => js(`(() => { const b = document.querySelector('[data-cause="root-missing"]'); return b ? b.textContent : null; })()`), 20000).catch(() => null);
    check("the window shows the banner", !!banner, banner);
    check("the banner offers to choose the folder again", await js(`!!document.querySelector('[data-action="root-missing.reselect"]')`));
    await wait(3000);
    process.stdout.write(`  page: ${await js("document.body.innerText.slice(0, 300).replace(/\\n/g, ' | ')")}\n`);
    const png = path.join(os.tmpdir(), "rx-root-missing-live.png");
    try {
      execFileSync("screencapture", ["-x", "-o", "-l", win.getMediaSourceId().split(":")[1], png]);
      process.stdout.write(`  screenshot: ${png}\n`);
    } catch (err) {
      process.stdout.write(`  (no screenshot: ${err.message})\n`);
    }
    fs.renameSync(gone, workspace);
  } catch (err) {
    check("the run completed", false, String(err && err.message ? err.message : err));
  }
  process.stdout.write(failures.length ? `${failures.length} check(s) failed\n` : "all checks passed\n");
  app.exit(failures.length ? 1 : 0);
}

app.whenReady().then(main);
