"use strict";
// Who owns the keyboard while the agent drives its browser. A person typing in
// the app page must still be typing there after the agent opens a page, shows
// it, clicks in it, types into it or asks for it to come to the front: the
// agent's commands arrive over the debugging protocol, and nothing it sends may
// move the focus out of the page the person is in. Run with `pnpm focus-live`;
// it needs no network and no kernel.
const { app, BrowserWindow } = require("electron");
const { BrowserProtocol } = require("../src/browserprotocol");
const { BrowserViews } = require("../src/browserviews");
const { checker, listen, wait } = require("./livekit");

const APP = `<!doctype html><title>app</title><textarea id="compose" autofocus></textarea>`;
const PAGE = `<!doctype html><title>page</title><input id="name"><button id="go" onclick="document.title='clicked'">Go</button>`;

const { check, failures } = checker();

async function main() {
  await app.whenReady();
  const page = await listen((_req, res) => {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(PAGE);
  });
  const win = new BrowserWindow({ width: 1000, height: 700, show: true, webPreferences: { contextIsolation: true } });
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

  const views = new BrowserViews({ win, kernelOrigin: "http://127.0.0.1:1" });
  const frames = [];
  const waiting = new Map();
  const protocol = new BrowserProtocol({
    createView: (spec) => views.create(spec),
    post: (frame) => {
      frames.push(frame);
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

  const where = async () => ({
    app: await win.webContents.executeJavaScript("document.activeElement && document.activeElement.id"),
    appFocused: win.webContents.isFocused(),
    viewFocused: [...views.entries.values()].some((e) => e.view.webContents.isFocused()),
  });
  const keepsTheKeyboard = async (what) => {
    const w = await where();
    check(`${what}: the composer keeps the keyboard`, w.app === "compose" && w.appFocused && !w.viewFocused, w);
  };

  protocol.receive({ conn: "c", open: "focus-live" });
  await keepsTheKeyboard("before the agent does anything");

  const created = await send({ method: "Target.createTarget", params: { url: page.url } });
  const targetId = created.result.targetId;
  const attached = await send({ method: "Target.attachToTarget", params: { targetId } });
  const session = attached.result.sessionId;
  await wait(1200);
  await keepsTheKeyboard("opening a page");

  views.show(targetId, { x: 300, y: 100, width: 600, height: 400 });
  await wait(500);
  await keepsTheKeyboard("showing it over the page");

  await send({ method: "Page.bringToFront" }, session);
  await wait(300);
  await keepsTheKeyboard("asking for the page to come to the front");

  await send({ method: "Input.dispatchMouseEvent", params: { type: "mousePressed", x: 40, y: 20, button: "left", clickCount: 1 } }, session);
  await send({ method: "Input.dispatchMouseEvent", params: { type: "mouseReleased", x: 40, y: 20, button: "left", clickCount: 1 } }, session);
  await wait(300);
  await keepsTheKeyboard("a click inside the agent's page");

  await send({ method: "Input.insertText", params: { text: "hello" } }, session);
  await send({ method: "Input.dispatchKeyEvent", params: { type: "keyDown", key: "a", text: "a" } }, session);
  await send({ method: "Input.dispatchKeyEvent", params: { type: "keyUp", key: "a" } }, session);
  await wait(300);
  await keepsTheKeyboard("typing into the agent's page");

  await send({ method: "Runtime.evaluate", params: { expression: "document.querySelector('#name').focus()" } }, session);
  await wait(300);
  await keepsTheKeyboard("the page focusing one of its own fields");

  await send({ method: "Page.navigate", params: { url: page.url + "/next" } }, session);
  await wait(1200);
  await keepsTheKeyboard("navigating the agent's page");

  views.hide();
  await wait(300);
  await keepsTheKeyboard("putting the page away");

  // The other half: a person who clicks into the agent's page is typing there,
  // and the agent navigating it again does not take that away from them.
  views.show(targetId, { x: 300, y: 100, width: 600, height: 400 });
  await wait(500);
  const theirs = [...views.entries.values()][0].view.webContents;
  // A click that reaches the page from the window, and the focus the platform
  // gives the page because of it; sendInputEvent alone does not move focus.
  theirs.sendInputEvent({ type: "mouseDown", x: 40, y: 20, button: "left", clickCount: 1 });
  theirs.focus();
  theirs.sendInputEvent({ type: "mouseUp", x: 40, y: 20, button: "left", clickCount: 1 });
  await wait(500);
  const chosen = await where();
  check("a click of the person's own into the agent's page gives it the keyboard", chosen.viewFocused && !chosen.appFocused, chosen);
  await send({ method: "Page.navigate", params: { url: page.url + "/again" } }, session);
  await wait(1200);
  const stays = await where();
  check("and the agent navigating it again does not pull the keyboard back", stays.viewFocused && !stays.appFocused, stays);

  win.webContents.focus();
  await wait(300);
  win.minimize();
  await wait(400);
  win.restore();
  await wait(600);
  await keepsTheKeyboard("the window being minimized and restored");

  page.server.close();
  app.exit(failures.length ? 1 : 0);
}

main().catch((err) => {
  process.stdout.write(`  FAIL focus-live crashed — ${err && err.stack}\n`);
  app.exit(1);
});
