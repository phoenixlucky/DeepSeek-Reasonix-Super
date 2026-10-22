// A step row carries a name, tags, an argument, a duration and a fold marker. When the column is narrow, the name,
// the argument and the tags give way (with their full text on hover); the duration and the fold marker never leave the row.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  for (const width of [320, 390, 768]) {
    const page = await browser.newPage({ viewport: { width, height: 844 }, colorScheme: scheme, reducedMotion: "reduce" });
    page.setDefaultTimeout(8000);
    page.on("pageerror", (e) => fails.push("page error: " + e.message));
    await page.goto(`${BASE}?pref=en`, { waitUntil: "networkidle" });
    await page.waitForSelector(".app");
    await page.evaluate(() => {
      window.__feed({ kind: "turn_started", authoredTurn: 1, msgIndex: 0, text: "go" });
      window.__feed({ kind: "text", text: "checking" });
      const call = (id, name, args, extra) => {
        window.__feed({ kind: "tool_dispatch", tool: { id, name, args, ...extra } });
        window.__feed({ kind: "tool_result", tool: { id, name, args, output: "ok", ...extra } });
      };
      call("t1", "mcp__time__get_current_time", '{"timezone":"Asia/Shanghai"}', { durationMs: 100 });
      call("t2", "web_fetch", '{"url":"https://platform.example.com/some/very/long/path/that/keeps/going/index.html"}', { durationMs: 600 });
      call("t3", "bash", '{"command":"cd /Users/me/Documents/project && for f in *.md; do head -20 $f; done"}', { durationMs: 1200 });
    });
    await page.waitForTimeout(500);
    await page.getByText("3 actions").first().click();
    await page.waitForTimeout(500);
    const out = await page.evaluate(() =>
      [...document.querySelectorAll(".activity-group .hl")]
        .filter((h) => h.getBoundingClientRect().width > 0)
        .flatMap((h) => {
          const edge = h.getBoundingClientRect().right;
          return [...h.children]
            .filter((c) => c.getBoundingClientRect().width > 0 && c.getBoundingClientRect().right > edge + 1)
            .map((c) => `${c.className.split(" ")[0]} ${Math.round(c.getBoundingClientRect().right - edge)}px past the row`);
        }),
    );
    check(`${scheme} ${width}px: nothing in a step row leaves the row`, out.length === 0, out.slice(0, 3).join("; "));
    await page.close();
  }
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
console.log("\nall ok");
