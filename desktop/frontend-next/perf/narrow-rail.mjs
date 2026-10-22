// The meter rail under the composer is one row. When it cannot hold every readout, a chip gives up its label and keeps
// its count; nothing may sit past the rail's right edge, where the window clips it.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  for (const width of [320, 390, 480, 600, 768]) {
    const page = await browser.newPage({ viewport: { width, height: 844 }, colorScheme: scheme, reducedMotion: "reduce" });
    page.setDefaultTimeout(8000);
    page.on("pageerror", (e) => fails.push("page error: " + e.message));
    await page.goto(`${BASE}?pref=en`, { waitUntil: "networkidle" });
    await page.waitForSelector(".app");
    await page.evaluate(() => {
      const args = '{"description":"review","subagent_type":"security-review"}';
      window.__feed({ kind: "turn_started", authoredTurn: 1, msgIndex: 0, text: "go" });
      window.__feed({ kind: "tool_dispatch", tool: { id: "a1", name: "task", profile: { name: "security-review" }, args } });
      window.__feed({ kind: "tool_result", tool: { id: "a1", name: "task", profile: { name: "security-review" }, args, output: "done" } });
      window.__feed({ kind: "text", text: "x" });
    });
    await page.waitForSelector(".studio-deckchip");
    await page.waitForTimeout(400);
    const m = await page.evaluate(() => {
      const rail = document.querySelector(".studio-meterrail").getBoundingClientRect();
      const live = (el) => {
        const r = el.getBoundingClientRect();
        return r.width > 0 && r.height > 0 ? r : null;
      };
      const out = [...document.querySelectorAll(".studio-meterrail > *, .studio-deck-anchor > .studio-deckchip")]
        .filter((el) => live(el) && live(el).right > rail.right + 1)
        .map((el) => `${String(el.className).split(" ")[0] || el.tagName} ${Math.round(live(el).right - rail.right)}px`);
      const chip = document.querySelector(".studio-deckchip");
      return { out, chipName: chip.getAttribute("aria-label") || chip.textContent.trim(), count: chip.querySelector("b")?.textContent };
    });
    check(`${scheme} ${width}px: nothing in the meter rail passes its right edge`, m.out.length === 0, m.out.slice(0, 3).join("; "));
    check(`${scheme} ${width}px: the deck chip still has a name and a count`, m.chipName.length > 0 && m.count !== undefined, JSON.stringify(m));
    await page.close();
  }
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
console.log("\nall ok");
