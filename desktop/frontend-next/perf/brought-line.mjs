// A call brought to the end of the transcript is the same row it is where it happened, drawn open: no box, no icon
// square, no vertical track — the status glyph, the title, the subject and the meta on one row over a hairline.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  const ctx = await browser.newContext({ locale: "zh-CN", viewport: { width: 1100, height: 900 }, colorScheme: scheme });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => fails.push("page error: " + e.message));
  await page.goto(BASE, { waitUntil: "networkidle" });
  await page.waitForSelector(".app");
  await page.waitForTimeout(600);
  await page.evaluate(() => {
    window.__feed({ kind: "turn_started", authoredTurn: 1, msgIndex: 0, text: "run" });
    window.__feed({ kind: "tool_dispatch", tool: { id: "t1", name: "bash", args: '{"command":"go test ./..."}' } });
    window.__feed({ kind: "tool_result", tool: { id: "t1", name: "bash", args: '{"command":"go test ./..."}', output: "ok\n", durationMs: 900 } });
    window.__feed({ kind: "text", text: "done" });
    window.__feed({ kind: "turn_done" });
  });
  await page.waitForTimeout(400);
  await page.evaluate(() => [...document.querySelectorAll('[data-action="tool.bring"]')].pop()?.click());
  await page.waitForSelector(".brought-item .call");
  await page.mouse.move(5, 5);
  await page.waitForTimeout(200);
  const m = await page.evaluate(() => {
    const call = document.querySelector(".brought-item .call");
    const g = call.querySelector(":scope > .g");
    if (!call.closest(".flow")) return { iconColumn: "outside .flow" };
    const c = call.querySelector(":scope > .c");
    const cs = getComputedStyle(c);
    return {
      iconColumn: g ? getComputedStyle(g).display : "absent",
      sides: [cs.borderTopWidth, cs.borderLeftWidth, cs.borderRightWidth].join(","),
      radius: cs.borderTopLeftRadius,
      bg: cs.backgroundColor,
      caption: !!document.querySelector(".brought-bar span"),
      open: !!call.querySelector("details[open]"),
    };
  });
  check(`${scheme}: no icon column or vertical track`, m.iconColumn === "none" || m.iconColumn === "absent", m.iconColumn);
  check(`${scheme}: no box around the row`, m.sides === "0px,0px,0px" && m.radius === "0px" && (m.bg === "rgba(0, 0, 0, 0)" || m.bg === "transparent"), JSON.stringify(m));
  check(`${scheme}: opened, with its caption above`, m.open && m.caption);
  await ctx.close();
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
console.log("\nall ok");
