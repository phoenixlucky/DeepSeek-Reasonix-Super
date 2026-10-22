// A `!` line refused because this connection cannot run commands comes back selected, so the next message replaces it and is sent.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const shotDir = process.env.SHOTS;
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  for (const width of [1280, 420]) {
    const tag = `${scheme} ${width}px`;
    const page = await browser.newPage({ viewport: { width, height: 900 }, locale: "zh-CN", colorScheme: scheme, reducedMotion: "reduce" });
    page.on("pageerror", (e) => fails.push("页面异常: " + e.message));
    await page.goto(`${BASE}?pref=zh&turns=2&netshell=1`, { waitUntil: "networkidle" });
    await page.waitForSelector(".app");
    const box = page.locator('[aria-label="任务输入"]');
    await box.waitFor();
    await box.fill("!ls -la");
    await box.press("Enter");
    await page.waitForFunction(() => (window.__sent ?? []).length === 1);
    await page.waitForFunction(() => { const t = document.querySelector('[aria-label="任务输入"]'); return t && t.value === "!ls -la" && t.selectionStart === 0 && t.selectionEnd === 7; });
    check(`${tag}: the refused command is back in the box, all of it selected`, true);
    const said = await page.evaluate(() => document.body.textContent.includes("HTTP 上不提供 shell 命令"));
    check(`${tag}: the refusal is worded`, said);
    if (shotDir) { await page.mouse.move(width - 3, 895); await page.waitForTimeout(300); await page.screenshot({ path: `${shotDir}/${scheme}-${width}-refused.png` }); }
    await page.keyboard.type("你好");
    check(`${tag}: typing replaces the command instead of joining it`, (await box.inputValue()) === "你好");
    await box.press("Enter");
    await page.waitForFunction(() => (window.__sent ?? []).length === 2);
    const sent = await page.evaluate(() => window.__sent);
    check(`${tag}: the next ordinary message is sent`, sent[1] === "你好", JSON.stringify(sent));
    await page.close();
  }
}
await browser.close();
if (fails.length) { console.log(`\n${fails.length} failed`); process.exit(1); }
console.log("\nall passed");
