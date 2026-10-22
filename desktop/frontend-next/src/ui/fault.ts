// A TypeError, RangeError or ReferenceError reaching an error notice is a defect
// in this page, not something the person did or the kernel refused, and the
// notice shows only its message. The stack goes to the console, and a short
// form of it is kept against the words shown so a report of that notice can say
// where it came from. Only the error's name, its message cut short and the
// frames' own file, line and column are kept: never a path, a host or anything
// a request carried.
const PAGE_FAULTS = [TypeError, RangeError, ReferenceError];
const MESSAGE_CAP = 200;
const FRAMES = 12;
const REPORTED_FRAMES = 6;
const KEPT = 20;

const details = new Map<string, string>();

// "fn (https://host/assets/a.js?x=1:12:3)" and "https://host/a.js:12:3" both
// become "fn (a.js:12:3)" / "a.js:12:3".
function frameOf(line: string): string {
  const m = /^\s*at (?:(.*?) \()?(.*?)\)?$/.exec(line);
  if (!m) return "";
  const where = /([^/\\?#:]+)(?:[?#][^:]*)?(?::(\d+):(\d+))?$/.exec(m[2] ?? "");
  if (!where) return "";
  const place = `${where[1]}${where[2] ? `:${where[2]}:${where[3]}` : ""}`;
  return m[1] ? `${m[1]} (${place})` : place;
}

export function noteFault(e: unknown, shown?: string): void {
  if (!(e instanceof Error) || !PAGE_FAULTS.some((kind) => e instanceof kind)) return;
  const frames = (e.stack ?? "").split("\n").filter((line) => /^\s+at /.test(line)).slice(0, FRAMES);
  console.error(`[reasonix] page fault ${e.name}: ${e.message.slice(0, MESSAGE_CAP)}\n${frames.join("\n")}`);
  if (!shown) return;
  const kept = frames.slice(0, REPORTED_FRAMES).map(frameOf).filter(Boolean).map((f) => `  at ${f}`);
  details.delete(shown);
  details.set(shown, [`${e.name}: ${e.message.slice(0, MESSAGE_CAP)}`, ...kept].join("\n"));
  if (details.size > KEPT) details.delete(details.keys().next().value as string);
}

/** What was kept for a notice's words, or "" when they were not a page fault's. */
export function faultDetail(shown: string): string {
  return details.get(shown) ?? "";
}
