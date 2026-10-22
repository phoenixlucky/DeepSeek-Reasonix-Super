"use strict";
// A local image the agent opens in its browser has to be on screen, not a blank
// panel under the image's own title. The page is opened the way the kernel opens
// one (Target.createTarget on a file: address), shown over the rectangle the
// panel reserves, and what the view actually drew is read back and judged.
// Run with `pnpm image-live`; it needs no network and no kernel.
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { app, BrowserWindow, nativeImage } = require("electron");
const { BrowserViews } = require("../src/browserviews");
const { BrowserProtocol } = require("../src/browserprotocol");
const { checker, listen, wait } = require("./livekit");

const { check, failures } = checker();

function picture(width, height, file) {
  const bitmap = Buffer.alloc(width * height * 4);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      const at = (y * width + x) * 4;
      bitmap[at] = (x * 255) / width;
      bitmap[at + 1] = (y * 255) / height;
      bitmap[at + 2] = 128;
      bitmap[at + 3] = 255;
    }
  }
  fs.writeFileSync(file, nativeImage.createFromBitmap(bitmap, { width, height }).toPNG());
}

// How much of what was drawn is neither the panel's own white nor transparent.
async function inked(view) {
  const shot = await view.webContents.capturePage();
  const { width, height } = shot.getSize();
  if (!width || !height) return { width, height, share: 0 };
  const bitmap = shot.toBitmap();
  let ink = 0;
  for (let i = 0; i < bitmap.length; i += 4) {
    const blank = bitmap[i + 3] === 0 || (bitmap[i] > 245 && bitmap[i + 1] > 245 && bitmap[i + 2] > 245);
    if (!blank) ink++;
  }
  return { width, height, share: ink / (bitmap.length / 4) };
}

async function main() {
  await app.whenReady();
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "rx-image-live-"));
  const small = path.join(dir, "small.png");
  const large = path.join(dir, "large.png");
  const html = path.join(dir, "page.html");
  picture(320, 180, small);
  picture(3200, 1800, large);
  fs.writeFileSync(html, `<!doctype html><title>page</title><body style="margin:0;background:#2a6"><h1>hello</h1>`);
  const web = await listen((req, res) => {
    res.writeHead(200, { "Content-Type": "image/png" });
    res.end(fs.readFileSync(large));
  });

  const win = new BrowserWindow({ width: 1280, height: 800, show: true });
  await win.loadURL("data:text/html;charset=utf-8," + encodeURIComponent("<body style='margin:0'>app"));
  await wait(500);
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
  const send = (message) => new Promise((resolve) => {
    const id = ++n;
    waiting.set(id, resolve);
    protocol.receive({ conn: "c", message: JSON.stringify({ id, ...message }) });
    setTimeout(() => resolve({ timedOut: true }), 5000);
  });
  protocol.receive({ conn: "c", open: "image-live" });

  const cases = [
    ["a small local image", "file://" + (process.platform === "win32" ? "/" : "") + small.replace(/\\/g, "/")],
    ["a 3200x1800 local image", "file://" + (process.platform === "win32" ? "/" : "") + large.replace(/\\/g, "/")],
    ["a local html page", "file://" + (process.platform === "win32" ? "/" : "") + html.replace(/\\/g, "/")],
    ["a 3200x1800 image over http", web.url + "/big.png"],
  ];
  for (const [name, url] of cases) {
    const created = await send({ method: "Target.createTarget", params: { url } });
    const targetId = created.result && created.result.targetId;
    if (!targetId) {
      check(`${name}: the agent got a page`, false, created);
      continue;
    }
    await wait(1500);
    const view = [...views.entries.values()].find((e) => e.view)?.view;
    const entry = views.entries.get(targetId);
    views.show(targetId, { x: 300, y: 100, width: 800, height: 500 });
    await wait(1200);
    const where = entry.view.getBounds();
    const drawn = await inked(entry.view);
    check(`${name}: it is placed over the reserved rectangle`, where.x === 300 && where.y === 100 && where.width === 800 && where.height === 500, where);
    check(`${name}: what the view drew is not blank`, drawn.share > 0.2, drawn);
    await send({ method: "Target.closeTarget", params: { targetId } });
    await wait(300);
    void view;
  }

  web.server.close();
  app.exit(failures.length ? 1 : 0);
}

main().catch((err) => {
  process.stdout.write(`  FAIL image-live crashed — ${err && err.stack}\n`);
  app.exit(1);
});
