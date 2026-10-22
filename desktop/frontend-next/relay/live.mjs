// A hosted page driving a real Studio through a stand-in relay. What a shipped
// page does over that link is the claim: the production bundle, the encrypted
// channel, the Studio's own host and a real kernel are all real; only the two
// Reasonix services between them are replaced (tools/relayfake).
import { chromium } from "playwright";
import { startStack, routeServices, until } from "./stack.mjs";

const need = (name) => {
  if (!process.env[name]) throw new Error(`${name} is not set; see relay/README.md`);
  return process.env[name];
};

const stack = await startStack({ hostBin: need("RELAY_HOST_BIN"), fakeBin: need("RELAY_FAKE_BIN"), dist: need("RELAY_DIST") });
const browser = await chromium.launch(process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {});
const failures = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "ok  " : "FAIL"} ${name}${detail ? ` - ${detail}` : ""}`);
  if (!ok) failures.push(name);
};

try {
  const context = await browser.newContext({ viewport: { width: 1300, height: 850 } });
  await routeServices(context, stack);
  const page = await context.newPage();
  const pageErrors = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.goto(stack.pageUrl());
  await page.evaluate(() => { window.__survivor = true; });
  await page.keyboard.press("Escape");
  const pane = await stack.api("/runtimes", "POST", { root: stack.workspace });
  check("a pane opens on the Studio", pane.status === 200, `status ${pane.status}`);

  const composer = page.getByLabel("Task input").first();
  await composer.waitFor({ timeout: 30_000 });
  const text = () => page.evaluate(() => document.body.innerText);
  const count = async (word) => ((await text()).match(new RegExp(word, "g")) ?? []).length;
  const send = async (message) => {
    await composer.fill(message);
    await composer.press("Enter");
  };

  await send("first");
  const live = await until("the reply to stream onto the page", async () => (await count("word11")) >= 1, 20_000).then(() => true, () => false);
  check("a reply streams onto the page over the relay", live, `provider saw ${stack.provider.requests.length} request(s)`);

  const dropped = await stack.ctl("drop-controllers", "POST");
  check("the relay drops the page's socket", dropped.dropped === 1, JSON.stringify(dropped));
  await until("the page to reconnect", async () => (await stack.ctl("state")).controllers === 1, 30_000).catch(() => {});
  await send("second");
  const after = await until("a reply after the reconnect", async () => (await count("word11")) >= 2, 40_000).then(() => true, () => false);
  check("a reply streams onto the page after the relay drops the socket", after);

  check("the page was never reloaded", await page.evaluate(() => window.__survivor === true));
  check("the page threw no error", pageErrors.length === 0, pageErrors.join(" | "));
} catch (error) {
  check("the scenario ran to the end", false, String(error));
} finally {
  await browser.close();
  await stack.stop();
}
process.exit(failures.length ? 1 : 0);
