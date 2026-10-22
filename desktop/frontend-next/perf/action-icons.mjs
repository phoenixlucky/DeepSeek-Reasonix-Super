// The row of actions under a reply and the row under the person's own message read as one family: the same
// button and the same icon size. A smaller icon under one of them is the kind of slip a reader sees at once.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?pref=zh&ws=2&sess=6&turns=2";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const size = (el) => el.evaluate((e) => {
  const b = e.getBoundingClientRect();
  return `${Math.round(b.width * 10) / 10}x${Math.round(b.height * 10) / 10}`;
});

const browser = await chromium.launch();
try {
  for (const scheme of ["light", "dark"]) {
    const page = await browser.newPage({ colorScheme: scheme, viewport: { width: 1200, height: 900 } });
    await page.goto(BASE, { waitUntil: "networkidle" });
    await page.waitForSelector(".user-acts");
    await page.waitForSelector(".call[data-k=\"say\"] .acts");
    const mine = page.locator(".user-acts .studio-icon");
    const reply = page.locator(".call[data-k=\"say\"] .acts .studio-icon");
    check(`${scheme}: both rows were found`, (await mine.count()) > 0 && (await reply.count()) > 0);
    const replyIcon = await size(reply.first());
    const replyButton = await size(page.locator(".call[data-k=\"say\"] .acts > button").first());
    for (let i = 0; i < await mine.count(); i++) {
      check(`${scheme}: icon ${i + 1} of the message's own row matches the reply's`, await size(mine.nth(i)) === replyIcon, `${await size(mine.nth(i))} vs ${replyIcon}`);
    }
    check(`${scheme}: the buttons are the same size`, await size(page.locator(".user-acts > button").first()) === replyButton);
    await page.close();
  }
} finally {
  await browser.close();
}
if (fails.length) {
  console.error(`\n${fails.length} failed: ${fails.join("; ")}`);
  process.exit(1);
}
console.log("\nall passed");
