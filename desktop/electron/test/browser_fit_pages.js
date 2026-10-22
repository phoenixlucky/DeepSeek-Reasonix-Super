"use strict";
// Local html previews, an oversized image document and a PDF in the built-in browser, each with and without an
// agent viewport: the view must be filled by the page, measured on the pixels, not just sized right.
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { app, BrowserWindow } = require("electron");
const { BrowserViews } = require("../src/browserviews");
const { checker } = require("./livekit");

app.setPath("userData", fs.mkdtempSync(path.join(os.tmpdir(), "rx-fit-pages-")));
const { check, failures } = checker();
const wait = (ms) => new Promise((r) => setTimeout(r, ms));
const PANEL = { x: 380, y: 60, width: 560, height: 620 };
const dir = fs.mkdtempSync(path.join(os.tmpdir(), "rx-pages-"));

function png(w, h, rgb) {
  const zlib = require("node:zlib");
  const crc = (buf) => { let c, t = []; for (let n = 0; n < 256; n++) { c = n; for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1; t[n] = c >>> 0; } let r = 0xffffffff; for (const b of buf) r = t[(r ^ b) & 255] ^ (r >>> 8); return (r ^ 0xffffffff) >>> 0; };
  const chunk = (type, data) => { const len = Buffer.alloc(4); len.writeUInt32BE(data.length); const body = Buffer.concat([Buffer.from(type), data]); const c = Buffer.alloc(4); c.writeUInt32BE(crc(body)); return Buffer.concat([len, body, c]); };
  const ihdr = Buffer.alloc(13); ihdr.writeUInt32BE(w, 0); ihdr.writeUInt32BE(h, 4); ihdr[8] = 8; ihdr[9] = 2;
  const row = Buffer.concat([Buffer.from([0]), Buffer.from(Array.from({ length: w }, () => rgb).flat())]);
  return Buffer.concat([Buffer.from("\x89PNG\r\n\x1a\n", "binary"), chunk("IHDR", ihdr), chunk("IDAT", zlib.deflateSync(Buffer.concat(Array.from({ length: h }, () => row)))), chunk("IEND", Buffer.alloc(0))]);
}
fs.writeFileSync(path.join(dir, "big.png"), png(3200, 1800, [37, 99, 235]));
fs.copyFileSync("/tmp/fl/preview.html", path.join(dir, "preview.html"));
fs.copyFileSync("/tmp/fl/page.pdf", path.join(dir, "page.pdf"));

// Share of the view's sampled pixels that are the near-white or light-grey empty background of an unfilled view.
async function emptyShare(view) {
  const img = await view.webContents.capturePage();
  const { width, height } = img.getSize();
  const bm = img.toBitmap();
  let empty = 0, blue = 0, n = 0;
  for (let y = 2; y < height - 2; y += Math.max(1, Math.floor(height / 40))) {
    for (let x = 2; x < width - 2; x += Math.max(1, Math.floor(width / 40))) {
      const i = (y * width + x) * 4;
      const b = bm[i], g = bm[i + 1], r = bm[i + 2];
      n++;
      if (Math.min(r, g, b) > 225 && Math.max(r, g, b) - Math.min(r, g, b) < 12) empty++;
      if (b > 180 && r < 90 && g < 150) blue++;
    }
  }
  return { share: empty / n, blue: blue / n, width, height };
}

app.whenReady().then(async () => {
  const win = new BrowserWindow({ width: 1000, height: 760, show: true });
  await win.loadURL("data:text/html,<body style='margin:0;background:%23ddd'>app");
  const views = new BrowserViews({ win, kernelOrigin: "http://127.0.0.1:1" });
  const cases = [
    ["local html preview, no override", "preview.html", null],
    ["local html preview, 1280x900 override", "preview.html", { width: 1280, height: 900 }],
    ["local html preview, oversized 3200x1800 override", "preview.html", { width: 3200, height: 1800 }],
    ["3200x1800 image document, no override", "big.png", null],
    ["3200x1800 image document, 3200x1800 override", "big.png", { width: 3200, height: 1800 }],
    ["PDF, no override", "page.pdf", null],
    ["PDF, 1280x900 override", "page.pdf", { width: 1280, height: 900 }],
  ];
  for (const [name, file, override] of cases) {
    const page = views.create({ partition: "p" + Math.random(), url: "file://" + path.join(dir, file), onEvent() {}, onClosed() {}, onPopup() {}, onDownload() {} });
    const entry = views.entries.get(page.targetId);
    await wait(file.endsWith(".pdf") ? 3500 : 1500);
    views.show(page.targetId, PANEL);
    await wait(500);
    if (override) await page.send("Emulation.setDeviceMetricsOverride", { ...override, deviceScaleFactor: 0, mobile: false });
    await wait(file.endsWith(".pdf") ? 2500 : 1200);
    const at = entry.view.getBounds();
    if (process.env.SHOTS) { fs.mkdirSync(process.env.SHOTS, { recursive: true }); fs.writeFileSync(path.join(process.env.SHOTS, name.replace(/[^a-z0-9]+/gi, "-") + ".png"), (await entry.view.webContents.capturePage()).toPNG()); }
    const m = await emptyShare(entry.view);
    const inside = at.x >= PANEL.x && at.y >= PANEL.y && at.x + at.width <= PANEL.x + PANEL.width && at.y + at.height <= PANEL.y + PANEL.height;
    const shown = file.endsWith(".html") ? m.share < 0.15 : m.share < 0.15 && m.blue >= (file.endsWith(".png") ? 0.5 : 0.05);
    const detail = `view ${JSON.stringify(at)}, empty ${(m.share * 100).toFixed(0)}%, page-coloured ${(m.blue * 100).toFixed(0)}% of ${m.width}x${m.height}`;
    console.log("   measured:", name, detail);
    check(`${name}: the view sits inside the panel and the page is drawn in it`, inside && shown, detail);
    views.close?.(page.targetId);
  }
  console.log(failures.length ? `\n${failures.length} failed` : "\nall passed");
  app.exit(failures.length ? 1 : 0);
});
