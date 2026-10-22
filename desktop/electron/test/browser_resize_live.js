"use strict";
// A page the agent lays out at a viewport wider than the pane has to stay whole for the
// person watching: the view shows all of it, scaled to the pane, not its left part.
const os = require("node:os");
const path = require("node:path");
const fs = require("node:fs");
const { execFileSync } = require("node:child_process");
const { app } = require("electron");
const { checker, listen, scriptedModel, seedHome, settledWindow, until, wait } = require("./livekit");

const PAGE = `<!doctype html><title>Wide</title><body style="margin:0">
<div style="width:100vw;height:100vh;background:linear-gradient(90deg,#e33,#33e);color:#fff;font:48px sans-serif">
<span style="position:absolute;left:12px;top:12px">LEFT</span>
<span style="position:absolute;right:12px;top:12px">RIGHT</span>
<button id="go" style="position:absolute;right:40px;top:300px;font:32px sans-serif" onclick="document.title='clicked'">Go</button></div>`;

const { check, failures } = checker();
const SIZE = { width: 1440, height: 1000 };

async function main() {
  const page = await listen((req, res) => {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(PAGE);
  });
  let calls = 0;
  const { handler, requests } = scriptedModel((tools) => {
    calls++;
    if (calls === 1) return { name: "browser_open", arguments: { url: `${page.url}/` } };
    if (calls === 2) return { name: "browser_act", arguments: { tab: "t1", steps: [{ action: "resize", ...SIZE }] } };
    if (calls === 3) return { name: "browser_read", arguments: { tab: "t1", what: "screenshot" } };
    if (calls === 4) {
      const ref = (tools.join("\n").match(/button "Go" \[(e\d+)\]/) || [])[1];
      return { name: "browser_act", arguments: { tab: "t1", steps: [{ action: "click", ref }] } };
    }
    return null;
  });
  const model = await listen(handler);
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "rx-resize-live-"));
  const workspace = fs.mkdtempSync(path.join(os.tmpdir(), "rx-resize-ws-"));
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
    await client.request("POST", `${runtimes[0].base}/submit`, { input: "open the wide page and lay it out wide" });
    await until("the scripted turn", async () => requests.length >= 5, 60000);
    const guests = () => win.contentView.children.filter((v) => v.webContents && v.webContents !== win.webContents);
    const guest = await until("a page view", async () => guests()[0]);
    await until("the page beside the conversation", async () => (guest.getBounds().x >= 0 ? true : null));
    await wait(1500);
    const bounds = guest.getBounds();
    const inner = await guest.webContents.executeJavaScript("[innerWidth, innerHeight]");
    check("the page is laid out at the viewport the agent asked for", inner[0] === SIZE.width && inner[1] === SIZE.height, inner);
    const draw = (name) => {
      const file = path.join(os.tmpdir(), name);
      try {
        execFileSync("screencapture", ["-x", "-o", "-l", win.getMediaSourceId().split(":")[1], file]);
        process.stdout.write(`  screenshot: ${file}\n`);
      } catch (err) {
        process.stdout.write(`  (no screenshot: ${err.message})\n`);
      }
    };
    const shown = await guest.webContents.capturePage();
    const size = shown.getSize();
    const px = shown.toBitmap();
    const at = (x, y) => { const i = (y * size.width + x) * 4; return { b: px[i], g: px[i + 1], r: px[i + 2] }; };
    const edge = at(size.width - 6, 6);
    check("the right edge of the page is what the view draws at its right edge", edge.b > edge.r, edge);
    check("the agent's click on a control at the right of the page landed", (await guest.webContents.getTitle()) === "clicked", await guest.webContents.getTitle());
    draw("rx-resize-live.png");
    const taken = await guest.webContents.debugger.sendCommand("Page.captureScreenshot", { format: "png" });
    const dims = require("electron").nativeImage.createFromBuffer(Buffer.from(taken.data, "base64")).getSize();
    process.stdout.write(`  agent screenshot ${JSON.stringify(dims)}\n`);
    check("a screenshot taken the way the agent takes one shows the page at the size it asked for", dims.width === SIZE.width && dims.height === SIZE.height, dims);
    process.stdout.write(`  bounds ${JSON.stringify(bounds)} inner ${JSON.stringify(inner)}\n`);
  } catch (err) {
    check("the run completed", false, String(err && err.message ? err.message : err));
  }
  process.stdout.write(failures.length ? `${failures.length} check(s) failed\n` : "all checks passed\n");
  app.exit(failures.length ? 1 : 0);
}

app.whenReady().then(main);
