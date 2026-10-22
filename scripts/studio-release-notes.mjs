#!/usr/bin/env node
// Renders release-notes/studio/<semver>.md for publication: each #N gains its
// pull request author, or for an issue the pull requests that closed it, and a
// 贡献者 list closes the notes. The source file is never rewritten.

import { readFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { annotateMarkdown, markdownRefs } from "./markdown-refs.mjs";
import {
  contributorLogins,
  creditSuffix,
  githubRefLookup,
  reportWarnings,
  resolveCredits,
  tokenFromEnvironment,
  unresolvedError,
} from "./release-credits.mjs";

// A second language rides in the same document after this line; the Versions
// panel and the GitHub page both read it (desktop/frontend-next notesLanguages.ts).
const ENGLISH_MARK = "<!-- lang:en -->";

export function renderStudioNotes(markdown, credits) {
  const body = annotateMarkdown(markdown, (ref) => creditSuffix(credits.get(ref))).trimEnd();
  const logins = contributorLogins(markdownRefs(markdown), credits).map((login) => `@${login}`);
  if (!logins.length) return `${body}\n`;
  const chinese = `## 贡献者\n\n感谢本版本的贡献者：${logins.join("、")}`;
  const at = body.indexOf(ENGLISH_MARK);
  if (at < 0) return `${body}\n\n${chinese}\n`;
  const first = body.slice(0, at).replace(/\n+---\s*$/, "").trimEnd();
  const english = `## Contributors\n\nThanks to this release's contributors: ${logins.join(", ")}`;
  return `${first}\n\n${chinese}\n\n---\n\n${body.slice(at)}\n\n${english}\n`;
}

async function main() {
  const [input, output] = process.argv.slice(2);
  if (!input || !output) throw new Error("usage: studio-release-notes.mjs <notes.md> <output.md>");
  const markdown = await readFile(input, "utf8");
  const refs = markdownRefs(markdown);
  let credits = new Map();
  let failures = [];
  if (refs.length) {
    const lookup = githubRefLookup({ token: tokenFromEnvironment() });
    let warnings;
    ({ credits, failures, warnings } = await resolveCredits(refs, lookup));
    reportWarnings(warnings);
  }
  await writeFile(output, renderStudioNotes(markdown, credits));
  if (failures.length) throw unresolvedError(failures);
  console.log(`Rendered ${input} to ${output}: ${new Set(refs).size} reference(s), ${credits.size} resolved`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(error.code ? `${error.code}: ${error.message}` : error.message);
    process.exitCode = 1;
  });
}
