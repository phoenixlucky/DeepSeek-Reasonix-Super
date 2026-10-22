import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { chmodSync, mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const pack = path.join(here, "package-cli-archive.sh");
const check = path.join(here, "check-cli-archives.py");

const TARGETS = [
  ["darwin", "amd64"], ["darwin", "arm64"], ["linux", "amd64"], ["linux", "arm64"], ["windows", "amd64"], ["windows", "arm64"],
];
const archiveName = (os, arch) => `reasonix-${os}-${arch}.${os === "windows" ? "zip" : "tar.gz"}`;

function fixture() {
  const root = mkdtempSync(path.join(tmpdir(), "cli-contents-"));
  const page = path.join(root, "dist");
  mkdirSync(path.join(page, "assets"), { recursive: true });
  writeFileSync(path.join(page, "index.html"), "<title>studio</title>");
  writeFileSync(path.join(page, "assets", "app.js"), "// page");
  const bin = path.join(root, "reasonix-built");
  writeFileSync(bin, "binary");
  chmodSync(bin, 0o700);
  const out = path.join(root, "out");
  mkdirSync(out);
  return { root, page, bin, out };
}

function build(f, targets = TARGETS) {
  for (const [os, arch] of targets) execFileSync("bash", [pack, os, arch, f.bin, f.page, f.out], { stdio: "pipe" });
  writeFileSync(path.join(f.out, "SHA256SUMS"), "");
}

const run = (f) => spawnSync("python3", [check, f.out], { encoding: "utf8" });

function listing(archive) {
  return archive.endsWith(".zip")
    ? execFileSync("unzip", ["-Z1", archive], { encoding: "utf8" })
    : execFileSync("tar", ["tzf", archive], { encoding: "utf8" });
}

test("every archive holds the executable at the root and the page under frontend-next/dist, and nothing else", () => {
  const f = fixture();
  try {
    build(f);
    for (const [os, arch] of TARGETS) {
      const names = listing(path.join(f.out, archiveName(os, arch))).split("\n").filter(Boolean).map((n) => n.replace(/\/$/, ""));
      const exe = os === "windows" ? "reasonix.exe" : "reasonix";
      assert.deepEqual(new Set(names), new Set([exe, "frontend-next", "frontend-next/dist", "frontend-next/dist/index.html", "frontend-next/dist/assets", "frontend-next/dist/assets/app.js"]), `${os}/${arch}`);
    }
    const done = run(f);
    assert.equal(done.status, 0, done.stderr);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("the executable keeps its mode and the page is not executable", () => {
  const f = fixture();
  try {
    build(f, [["linux", "amd64"]]);
    const verbose = execFileSync("tar", ["tzvf", path.join(f.out, "reasonix-linux-amd64.tar.gz")], { encoding: "utf8" });
    assert.match(verbose, /^-rwxr-xr-x .* reasonix$/m);
    assert.match(verbose, /^-rw-r--r-- .* frontend-next\/dist\/index\.html$/m);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("the packer refuses a page that is not built, a missing binary and a page holding a link", () => {
  const f = fixture();
  try {
    rmSync(path.join(f.page, "index.html"));
    assert.notEqual(spawnSync("bash", [pack, "linux", "amd64", f.bin, f.page, f.out]).status, 0);
    writeFileSync(path.join(f.page, "index.html"), "x");
    assert.notEqual(spawnSync("bash", [pack, "linux", "amd64", path.join(f.root, "nope"), f.page, f.out]).status, 0);
    symlinkSync("/etc/passwd", path.join(f.page, "link"));
    assert.notEqual(spawnSync("bash", [pack, "linux", "amd64", f.bin, f.page, f.out]).status, 0);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

// Repack one archive with an extra entry and see the checker refuse the set.
function tamper(f, os, arch, extra) {
  const staging = path.join(f.root, "tamper");
  rmSync(staging, { recursive: true, force: true });
  mkdirSync(staging);
  const archive = path.join(f.out, archiveName(os, arch));
  if (os === "windows") execFileSync("unzip", ["-q", archive, "-d", staging]);
  else execFileSync("tar", ["xzf", archive, "-C", staging]);
  extra(staging);
  rmSync(archive);
  if (os === "windows") execFileSync("zip", ["-qry", archive, "reasonix.exe", "frontend-next", ...(extraTop.length ? extraTop : [])], { cwd: staging });
  else execFileSync("tar", ["czf", archive, "-C", staging, "reasonix", "frontend-next", ...extraTop], {});
  extraTop.length = 0;
}
const extraTop = [];

test("the checker refuses a second file named like the executable, even inside the page", () => {
  const f = fixture();
  try {
    build(f);
    tamper(f, "linux", "amd64", (dir) => writeFileSync(path.join(dir, "frontend-next", "dist", "reasonix"), "x"));
    const done = run(f);
    assert.notEqual(done.status, 0);
    assert.match(done.stderr, /named like the executable/);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("the checker refuses a top-level extra file, a link, and a page without index.html", () => {
  for (const [label, extra, want] of [
    ["extra top-level file", (dir) => { writeFileSync(path.join(dir, "notes.txt"), "x"); extraTop.push("notes.txt"); }, /unexpected entry/],
    ["link in the page", (dir) => symlinkSync("/etc/passwd", path.join(dir, "frontend-next", "dist", "link")), /not a plain file/],
    ["no index", (dir) => rmSync(path.join(dir, "frontend-next", "dist", "index.html")), /no frontend-next\/dist\/index\.html/],
  ]) {
    const f = fixture();
    try {
      build(f);
      tamper(f, "linux", "arm64", extra);
      const done = run(f);
      assert.notEqual(done.status, 0, label);
      assert.match(done.stderr, want, label);
    } finally {
      rmSync(f.root, { recursive: true, force: true });
    }
  }
});

test("the checker refuses a zip that carries a second exe, and a set with a file missing", () => {
  const f = fixture();
  try {
    build(f);
    tamper(f, "windows", "amd64", (dir) => writeFileSync(path.join(dir, "frontend-next", "dist", "reasonix.exe"), "x"));
    assert.match(run(f).stderr, /named like the executable/);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
  const g = fixture();
  try {
    build(g);
    rmSync(path.join(g.out, "SHA256SUMS"));
    assert.match(run(g).stderr, /no .*SHA256SUMS/);
  } finally {
    rmSync(g.root, { recursive: true, force: true });
  }
});
