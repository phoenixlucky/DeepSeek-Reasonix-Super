"use strict";
// The agent opens a local image in the built-in browser, through the real kernel
// and the real window. The tab has to show the picture: a view over the
// rectangle the panel reserved whose pixels are the image, small or 3200x1800.
// Run with `pnpm image-pane-live`; it needs no network.
const os = require("node:os");
const path = require("node:path");
const fs = require("node:fs");
const { app, nativeImage } = require("electron");
const { checker, scriptedModel, listen, seedHome, settledWindow, until, wait } = require("./livekit");

const { check, failures } = checker();
const script = { url: "" };

function picture(width, height, file) {
  const bitmap = Buffer.alloc(width * height * 4);
  for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
    const at = (y * width + x) * 4;
    bitmap[at] = (x * 255) / width; bitmap[at + 1] = (y * 255) / height; bitmap[at + 2] = 128; bitmap[at + 3] = 255;
  }
  fs.writeFileSync(file, nativeImage.createFromBitmap(bitmap, { width, height }).toPNG());
}

async function main() {
  // The kernel only lets the agent open a local file inside the workspace.
  const workspace = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "rx-image-ws-")));
  const small = path.join(workspace, "small.png");
  const large = path.join(workspace, "ode_solution.png");
  picture(320, 180, small);
  picture(3200, 1800, large);
  const fileURL = (p) => "file://" + (process.platform === "win32" ? "/" : "") + p.replace(/\\/g, "/");

  let calls = 0;
  const model = await listen(scriptedModel((tools) => {
    if (calls >= 1) return null;
    calls++;
    return { name: "browser_open", arguments: { url: script.url } };
  }).handler);
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "rx-image-home-"));
  process.env.REASONIX_HOME = home;
  seedHome(home, model.url, workspace);
  const { current } = require("../src/main.js");
  const win = await settledWindow();
  const { client } = current();
  const runtimes = await until("a pane", async () => {
    const list = await client.json("GET", "/runtimes");
    return Array.isArray(list) && list.length ? list : null;
  });
  const base = runtimes[0].base;
  const guests = () => win.contentView.children.filter((v) => v.webContents && v.webContents !== win.webContents);

  for (const [name, file] of [["a small local image", small], ["a 3200x1800 local image", large]]) {
    script.url = fileURL(file);
    calls = 0;
    const before = guests().length;
    await client.request("POST", `${base}/submit`, { input: `show ${name}` });
    const guest = await until(`${name}: a page view`, async () => guests()[guests().length - 1], 60000);
    await until(`${name}: the page beside the conversation`, async () => (guest.getBounds().x >= 0 ? true : null), 30000).catch(() => null);
    await wait(1500);
    const b = guest.getBounds();
    const shot = await guest.webContents.capturePage();
    const size = shot.getSize();
    const bitmap = size.width ? shot.toBitmap() : Buffer.alloc(0);
    let ink = 0;
    for (let i = 0; i < bitmap.length; i += 4) if (!(bitmap[i + 3] === 0 || (bitmap[i] > 245 && bitmap[i + 1] > 245 && bitmap[i + 2] > 245))) ink++;
    const share = bitmap.length ? ink / (bitmap.length / 4) : 0;
    check(`${name}: the view is on screen, beside the conversation`, b.x >= 0 && b.width > 100 && b.height > 100, b);
    check(`${name}: the title is the image's own`, guest.webContents.getTitle().includes(path.basename(file)), guest.webContents.getTitle());
    check(`${name}: what the view drew is the picture, not a blank`, share > 0.2, { share, size, bounds: b, scale: require("electron").screen.getPrimaryDisplay().scaleFactor });
    void before;
  }
  app.exit(failures.length ? 1 : 0);
}

app.whenReady().then(main).catch((err) => {
  process.stdout.write(`  FAIL image-pane-live crashed — ${err && err.stack}\n`);
  app.exit(1);
});
