"use strict";
// A person in the middle of an IME composition — the pinyin letters still
// underlined, the candidate list open — must not lose it when the agent's browser
// loads, navigates, reloads or is shown. Losing it is not a lost focus that comes
// back: the composition is committed as the raw letters and the word they were
// spelling is gone. Run with `pnpm ime-live`; it needs no network and no kernel.
const { app, BrowserWindow } = require("electron");
const { BrowserViews, LEAVE_LIMIT_MS } = require("../src/browserviews");
const { BrowserProtocol } = require("../src/browserprotocol");
const { checker, listen, wait } = require("./livekit");

const APP = `<!doctype html><title>app</title><textarea id="compose" autofocus></textarea>
<script>window.__ime = { start: 0, update: 0, end: 0, blur: 0 };
const t = document.getElementById("compose");
t.addEventListener("compositionstart", () => __ime.start++);
t.addEventListener("compositionupdate", () => __ime.update++);
t.addEventListener("compositionend", () => __ime.end++);
t.addEventListener("blur", () => __ime.blur++);</script>`;
const PAGE = `<!doctype html><title>page</title><input id="name"><p>hello</p>`;

const { check, failures } = checker();

async function main() {
  await app.whenReady();
  const page = await listen((req, res) => {
    // A server that takes the connection and never answers.
    if (req.url === "/hang") return;
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(PAGE);
  });
  const win = new BrowserWindow({ width: 1000, height: 700, show: true });
  await win.loadURL("data:text/html;charset=utf-8," + encodeURIComponent(APP));
  win.show();
  app.focus({ steal: true });
  win.focus();
  win.webContents.focus();
  await wait(800);
  if (!win.webContents.isFocused()) {
    process.stdout.write("  SKIP this desktop would not give the test window the keyboard, so there is nothing to keep\n");
    app.exit(0);
    return;
  }
  const dbg = win.webContents.debugger;
  dbg.attach("1.3");

  const views = new BrowserViews({ win, kernelOrigin: "http://127.0.0.1:1" });
  const waiting = new Map();
  const protocol = new BrowserProtocol({
    createView: (spec) => views.create(spec),
    post: (frame) => {
      const id = frame.message && frame.message.id;
      if (id !== undefined && waiting.has(id)) waiting.get(id)(frame.message);
    },
  });
  let n = 0;
  const send = (message, sessionId) => new Promise((resolve) => {
    const id = ++n;
    waiting.set(id, resolve);
    protocol.receive({ conn: "c", message: JSON.stringify({ id, ...message, ...(sessionId ? { sessionId } : {}) }) });
    setTimeout(() => resolve({ timedOut: true }), 4000);
  });
  const ime = () => win.webContents.executeJavaScript("JSON.stringify({ ...window.__ime, value: document.getElementById('compose').value })").then(JSON.parse);
  const compose = async (text) => {
    await win.webContents.executeJavaScript("window.__ime.start = window.__ime.update = window.__ime.end = window.__ime.blur = 0; document.getElementById('compose').value = ''; document.getElementById('compose').focus()");
    await dbg.sendCommand("Input.imeSetComposition", { text, selectionStart: text.length, selectionEnd: text.length });
    await wait(150);
  };
  const keepsComposing = async (what) => {
    const s = await ime();
    check(`${what}: the composition is still open`, s.start >= 1 && s.end === 0, s);
  };

  protocol.receive({ conn: "c", open: "ime-live" });

  await compose("nihao");
  await keepsComposing("before the agent does anything");

  const created = await send({ method: "Target.createTarget", params: { url: page.url } });
  const targetId = created.result.targetId;
  const session = (await send({ method: "Target.attachToTarget", params: { targetId } })).result.sessionId;
  await wait(1500);
  await keepsComposing("opening a page");

  await compose("zhongwen");
  views.show(targetId, { x: 300, y: 100, width: 600, height: 400 });
  await wait(600);
  await keepsComposing("showing it");

  await compose("ceshi");
  await send({ method: "Page.navigate", params: { url: page.url + "/next" } }, session);
  await wait(1500);
  await keepsComposing("navigating it");

  await compose("shuaxin");
  await send({ method: "Page.reload" }, session);
  await wait(1500);
  await keepsComposing("reloading it");

  await compose("dianji");
  await send({ method: "Input.dispatchMouseEvent", params: { type: "mousePressed", x: 40, y: 20, button: "left", clickCount: 1 } }, session);
  await send({ method: "Input.dispatchMouseEvent", params: { type: "mouseReleased", x: 40, y: 20, button: "left", clickCount: 1 } }, session);
  await send({ method: "Input.insertText", params: { text: "x" } }, session);
  await wait(500);
  await keepsComposing("the agent clicking and typing in it");

  await compose("guaqi");
  // Not awaited: the protocol answers a navigation only when the page does.
  void send({ method: "Page.navigate", params: { url: page.url + "/hang" } }, session);
  await wait(500);
  const out = !win.contentView.children.includes([...views.entries.values()][0].view);
  check("a navigation that has just started holds the page out of the window", out);
  await wait(LEAVE_LIMIT_MS + 1000);
  const back = win.contentView.children.includes([...views.entries.values()][0].view);
  check("one that never answers does not keep it out for good", back);
  await keepsComposing("and the composition is still open while it hangs");

  page.server.closeAllConnections?.();
  page.server.close();
  app.exit(failures.length ? 1 : 0);
}

main().catch((err) => {
  process.stdout.write(`  FAIL ime-live crashed — ${err && err.stack}\n`);
  app.exit(1);
});
