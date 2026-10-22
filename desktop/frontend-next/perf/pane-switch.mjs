// Tab-switch and sidebar-filter cost with 1/2/4/8 panes that each hold a long transcript. A measurement, not a guard:
// the numbers depend on the machine, so it prints a table instead of failing. CPU=4 emulates a slow laptop.
import { chromium } from "playwright";

const CPU = Number(process.env.CPU ?? 4);
const TURNS = Number(process.env.TURNS ?? 60);
const REPS = Number(process.env.REPS ?? 3);
const b = await chromium.launch();
const rows = [];
const pct = (a, p) => { const s = [...a].sort((x, y) => x - y); return s[Math.min(s.length - 1, Math.floor(s.length * p))]; };
for (const n of [1, 2, 4, 8]) {
  const ctx = await b.newContext({ locale: "zh-CN", viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  const cdp = await ctx.newCDPSession(page);
  await page.goto(`${process.env.PERF_URL ?? "http://localhost:4399/perf.html"}?panes=${n}`, { waitUntil: "networkidle" });
  await page.waitForSelector(".app"); await page.waitForTimeout(800);
  await page.evaluate(async (k) => {
    const frame = () => new Promise((r) => requestAnimationFrame(r));
    for (let i = 0; i < k; i++) {
      window.__feed({ kind: "turn_started" });
      const id = `t${i}`;
      window.__feed({ kind: "tool_dispatch", tool: { id, name: "bash", args: JSON.stringify({ command: `go test ./pkg${i}/...` }) } });
      window.__feed({ kind: "tool_result", tool: { id, name: "bash", args: JSON.stringify({ command: `go test ./pkg${i}/...` }), output: "ok\n".repeat(20) } });
      window.__feed({ kind: "text", text: `第 ${i} 段回答。\n\n- 要点一\n- 要点二\n\n\`\`\`go\nfunc f${i}() {}\n\`\`\`\n` });
      window.__feed({ kind: "message" });
      window.__feed({ kind: "turn_done" });
      if (i % 10 === 9) await frame();
    }
    await frame();
  }, TURNS);
  await page.waitForTimeout(800);
  if (CPU > 1) await cdp.send("Emulation.setCPUThrottlingRate", { rate: CPU });
  await cdp.send("Performance.enable");
  const nodes = (await cdp.send("Performance.getMetrics")).metrics.find((m) => m.name === "Nodes")?.value;
  // tab switch: click each session row in turn, time until two frames have painted
  const switches = [];
  const m0 = Object.fromEntries((await cdp.send("Performance.getMetrics")).metrics.map((m) => [m.name, m.value]));
  for (let r = 0; r < REPS; r++) {
    const count = await page.locator(".sessrow").count();
    for (let i = 0; i < count; i++) {
      const ms = await page.evaluate(async (i) => {
        const el = document.querySelectorAll(".sessrow")[i];
        const t0 = performance.now();
        el.click();
        await new Promise((res) => requestAnimationFrame(() => requestAnimationFrame(res)));
        return performance.now() - t0;
      }, i);
      switches.push(ms);
      await page.waitForTimeout(80);
    }
  }
  const m1 = Object.fromEntries((await cdp.send("Performance.getMetrics")).metrics.map((m) => [m.name, m.value]));
  const per = (k) => (((m1[k] - m0[k]) * 1000) / switches.length).toFixed(0);
  // sidebar: type into the rail find box and measure each keystroke to paint
  const keys = [];
  const box = page.locator(".wsfind input");
  await page.evaluate(() => document.querySelector(".wsfind input")?.focus());
  for (const ch of "perf") {
    const ms = await page.evaluate(async (ch) => {
      const input = document.querySelector(".wsfind input");
      const t0 = performance.now();
      const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set;
      set.call(input, input.value + ch);
      input.dispatchEvent(new Event("input", { bubbles: true }));
      await new Promise((res) => requestAnimationFrame(() => requestAnimationFrame(res)));
      return performance.now() - t0;
    }, ch);
    keys.push(ms);
  }
  rows.push({ N: n, 节点: nodes, "切换 p50": pct(switches, 0.5).toFixed(0), "切换 p95": pct(switches, 0.95).toFixed(0), "切换 max": Math.max(...switches).toFixed(0), "脚本/次": per("ScriptDuration"), "样式/次": per("RecalcStyleDuration"), "布局/次": per("LayoutDuration"), "侧栏筛选 p50": pct(keys, 0.5).toFixed(0), "侧栏筛选 max": Math.max(...keys).toFixed(0) });
  await ctx.close();
}
await b.close();
console.log(`CPU 降频 ${CPU}x，每窗格 ${TURNS} 轮历史（含 bash 工具卡），切换 ${REPS} 轮；单位 ms（点击到两帧之后）\n`);
console.table(rows);
