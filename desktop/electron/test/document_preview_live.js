"use strict";
const os = require("node:os");
const path = require("node:path");
const fs = require("node:fs");
const { app } = require("electron");
const { checker, scriptedModel, listen, seedHome, settledWindow, until } = require("./livekit");
const { pdf } = require("../../frontend-next/perf/document-fixtures.cjs");
const { check, failures } = checker();

async function main() {
  const workspace = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "rx-documents-")));
  fs.writeFileSync(path.join(workspace, "方案.md"), "# Delivery report\n\nThe file opens in the workbench.");
  fs.writeFileSync(path.join(workspace, "table.csv"), 'id,value\n001,"hello\nworld"\n002,=SUM(A1:A2)');
  fs.writeFileSync(path.join(workspace, "page.html"), '<h1>Static document</h1><script>parent.__unsafe=true</script>');
  fs.writeFileSync(path.join(workspace, "report.pdf"), pdf());
  let issued = false;
  const provider = scriptedModel(() => {
    if (issued) return null;
    issued = true;
    return { name: "present_files", arguments: { files: [
      { path: "方案.md", description: "Delivery report" },
      { path: "table.csv", description: "Data" },
      { path: "page.html", description: "Static document" },
      { path: "report.pdf", description: "PDF document" },
    ] } };
  });
  const model = await listen(provider.handler);
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "rx-document-home-"));
  process.env.REASONIX_HOME = home;
  seedHome(home, model.url, workspace);
  const configPath = path.join(home, "config.toml");
  fs.writeFileSync(configPath, fs.readFileSync(configPath, "utf8").replace("[desktop]", '[desktop]\nlanguage = "zh"') + "\n[tools]\npresent_files = true\n");
  const { current } = require("../src/main.js");
  const win = await settledWindow();
  win.webContents.session.webRequest.onBeforeRequest((details, callback) => {
    const url = new URL(details.url);
    callback({ cancel: ["http:", "https:"].includes(url.protocol) && url.hostname !== "127.0.0.1" && url.hostname !== "localhost" });
  });
  const { client } = current();
  const runtimes = await until("workspace runtime", async () => {
    const rows = await client.json("GET", "/runtimes");
    return rows?.length ? rows : null;
  });
  const base = runtimes[0].base;
  const js = (code) => win.webContents.executeJavaScript(code);
  await until("visible application", () => js("!document.getElementById('boot') && !!document.querySelector('.compose')"));
  await client.request("POST", `${base}/submit`, { input: "Present the four existing files: 方案.md, table.csv, page.html, report.pdf." });
  await until("delivery cards", () => js("document.querySelectorAll('.delivery-open').length === 4"), 60000);
  check("real tool execution presents four files", true);
  await js("document.querySelector('.delivery-open').click()");
  await until("rendered Markdown", () => js("document.querySelector('.workbench-read h1')?.textContent === 'Delivery report'"));
  check("card opens the actual workspace document", true);
  const history = await client.json("GET", `${base}/history`);
  check("history includes confirmed file deliveries", history.some((row) => row.deliveries?.length === 4));
  await js("[...document.querySelectorAll('.delivery-open')].find(b => b.textContent.includes('table.csv')).click()");
  await until("CSV table from raw host bytes", () => js("document.querySelector('.document-table')?.textContent.includes('001')"));
  check("packaged CSV worker renders literal cells", await js("document.querySelector('.document-table').textContent.includes('=SUM(A1:A2)')"));
  await js("[...document.querySelectorAll('.delivery-open')].find(b => b.textContent.includes('page.html')).click()");
  await until("static HTML frame", () => js("!!document.querySelector('iframe[title=\"HTML 静态预览\"]')"));
  check("static HTML keeps scripts disabled", await js("!document.querySelector('iframe[title=\"HTML 静态预览\"]').sandbox.contains('allow-scripts') && !window.__unsafe"));
  await js("[...document.querySelectorAll('.delivery-open')].find(b => b.textContent.includes('report.pdf')).click()");
  await until("offline PDF worker and text layer", () => js("document.querySelector('.textLayer')?.textContent.includes('Report preview')"));
  check("production PDF worker and fonts work without external network", true);
  fs.writeFileSync("/tmp/reasonix-native-document-preview.png", (await win.webContents.capturePage()).toPNG());
  const loaded = new Promise((resolve) => win.webContents.once("did-finish-load", resolve));
  win.webContents.reload();
  await loaded;
  await until("visible application after reload", () => js("!document.getElementById('boot') && !!document.querySelector('.compose')").catch(() => false));
  await until("restored delivery cards", () => js("document.querySelectorAll('.delivery-open').length === 4").catch(() => false));
  check("delivery cards survive window reload", true);
  await js("document.querySelector('.delivery-open').click()");
  await until("reopened report", () => js("document.querySelector('.workbench-read h1')?.textContent === 'Delivery report'"));
  check("restored card opens the same file", true);
  model.server.close();
  app.exit(failures.length ? 1 : 0);
}

app.whenReady().then(main).catch((err) => {
  process.stderr.write(`${err.stack || err}\n`);
  app.exit(1);
});
