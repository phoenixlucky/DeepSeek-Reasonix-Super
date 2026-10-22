// A real page on a real Studio while its network flaps. Nine panes stream on one
// multiplexed connection while the browser keeps going offline and back across
// three turns, so the flaps land on turn starts and ends. What is judged is what
// the person sees on the pane they have open, read without navigating: raw frames
// are not the claim, because a page may recover a lost one from a snapshot.
import fs from "node:fs";
import path from "node:path";
import { chromium } from "playwright";
import { startStack, until } from "./stack.mjs";

const need = (name) => {
  if (!process.env[name]) throw new Error(`${name} is not set; see relay/README.md`);
  return process.env[name];
};
const PANES = 9;
const TURNS = 3;
const WORDS = 15;

const stack = await startStack({ hostBin: need("RELAY_HOST_BIN"), fakeBin: need("RELAY_FAKE_BIN"), dist: need("RELAY_DIST"), servePage: true, words: WORDS, gapMs: 100 });
const browser = await chromium.launch(process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {});
const failures = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "ok  " : "FAIL"} ${name}${detail ? ` - ${detail}` : ""}`);
  if (!ok) failures.push(name);
};

try {
  const panes = [];
  for (let i = 0; i < PANES; i++) {
    const root = path.join(stack.workspace, `p${i}`);
    fs.mkdirSync(root);
    panes.push((await stack.api("/runtimes", "POST", { root })).json);
  }
  const context = await browser.newContext({ viewport: { width: 1300, height: 1400 } });
  await context.addCookies([{ name: "reasonix_token", value: stack.hostToken, url: stack.hostOrigin }]);
  await context.addInitScript(() => {
    const Native = window.EventSource;
    window.__streams = [];
    window.EventSource = class extends Native {
      constructor(url, options) {
        super(url, options);
        window.__streams.push(String(url));
      }
    };
  });
  const page = await context.newPage();
  if (process.env.RELAY_DEBUG) {
    page.on("console", (m) => console.log("[console]", m.type(), m.text().slice(0, 160)));
    page.on("pageerror", (e) => console.log("[pageerror]", e.message));
    page.on("framenavigated", (f) => console.log("[navigated]", f.url()));
  }
  await page.goto(stack.hostOrigin + "/");
  await page.keyboard.press("Escape");
  await until("the sidebar to list every project", async () => (await page.getByText(/^p\d$/).count()) >= PANES, 30_000).catch(() => {});

  const turnOn = async (turn) => {
    await Promise.all(panes.map((p, i) => stack.api(`${p.base}/submit`, "POST", { input: `pane${i} turn ${turn}` })));
    return until(`turn ${turn} to finish on every pane`, async () => {
      const states = await Promise.all(panes.map(async (p) => (await stack.api(`${p.base}/status`)).json));
      return states.every((st) => st && st.running === false);
    }, 60_000).then(() => true, () => false);
  };
  let settled = await turnOn(0);
  await page.getByText(/^pane0 turn/).first().click({ timeout: 15_000 }).catch(async () => {
    await page.getByText("p0", { exact: true }).first().click();
    await page.getByText(/^pane0 turn/).first().click();
  });
  await page.waitForTimeout(1500);
  if (process.env.RELAY_SHOT) await page.screenshot({ path: process.env.RELAY_SHOT.replace('.png', '-before.png') });

  let flapping = true;
  const flaps = (async () => {
    while (flapping) {
      await context.setOffline(true);
      await new Promise((resolve) => setTimeout(resolve, 200));
      await context.setOffline(false);
      await new Promise((resolve) => setTimeout(resolve, 400));
    }
  })();
  for (let turn = 1; turn <= TURNS && settled; turn++) settled = await turnOn(turn);
  flapping = false;
  await flaps;
  await context.setOffline(false);
  check("every pane finished every turn on the Studio", settled);

  const streams = await page.evaluate(() => window.__streams);
  check("no pane held a stream of its own", streams.every((u) => !/\/rt\/.*\/events/.test(u)), streams.filter((u) => /\/rt\//.test(u)).join(" "));

  // The open pane is read where it stands, with no navigation: opening a pane
  // reloads it from its transcript, which would hide a live stream that stopped.
  // Recovery after the last flap has a bounded time to finish.
  const read = () => page.evaluate(() => document.body.innerText);
  const done = await until("the open pane to settle", async () => /\bIdle\b|Done/.test(await read()), 30_000).then(() => true, () => false);
  const text = await read();
  const replies = (text.match(new RegExp(`word${WORDS - 1}`, "g")) ?? []).length;
  check("the open pane shows every reply it was sent while the network flapped", replies >= TURNS + 1, `${replies} of ${TURNS + 1} replies`);
  check("the open pane returns to idle", done);

  // The sidebar counts what is running from the page's own state of every pane,
  // open or not; the Studio, the shared stream's metrics and the tree already agree
  // that nothing runs, so a count that stays up is the page not recovering.
  const live = () => page.evaluate(() => document.querySelector('.studio-session-segments button[data-value="live"] b')?.textContent?.trim() ?? "0");
  const quiet = await until("the sidebar's running count to reach zero", async () => (await live()) === "0", 45_000).then(() => true, () => false);
  const stuck = quiet ? "" : await page.evaluate(() => [...document.querySelectorAll(".sessrow[data-run]")].map((r) => `${r.textContent.trim().slice(0, 18)}:${r.getAttribute("data-run")}`).join(" | "));
  check("the sidebar's running count settles to zero once every turn has finished", quiet, quiet ? "" : `reads ${await live()}; rows ${stuck}`);
  if (process.env.RELAY_SHOT) {
    await page.mouse.move(700, 700);
    await page.waitForTimeout(2500);
    await page.screenshot({ path: process.env.RELAY_SHOT });
  }
} catch (error) {
  check("the scenario ran to the end", false, String(error));
} finally {
  await browser.close();
  await stack.stop();
}
process.exit(failures.length ? 1 : 0);
