import { expect, it } from "vitest";
import { fileAppearance } from "./FileTypeIcon";

it.each([
  ["方案.MDX", "markdown"], ["D:\\报告\\REPORT.PDF", "pdf"], ["data.csv", "spreadsheet"],
  ["budget.xlsx", "spreadsheet"], ["合同.docx", "document"], ["slides.key", "presentation"],
  ["index.html", "html"], ["photo.HEIC", "image"], ["art.svg", "vector"],
  ["design.psd", "design"], ["song.flac", "audio"], ["demo.mov", "video"],
  ["backup.tar.gz", "archive"], ["font.woff2", "font"], ["db.sqlite3", "database"],
  ["data.jsonl", "data"], ["feed.xml", "markup"], ["theme.scss", "stylesheet"],
  [".env.production", "config"], ["config.yaml", "config"], ["start.ps1", "script"],
  ["app.tsx", "code"], ["analysis.py", "code"], ["notebook.ipynb", "notebook"],
  ["server.log", "text"], ["book.epub", "ebook"], ["app.dmg", "package"],
  ["module.wasm", "binary"], ["mesh.glb", "model"], ["drawing.dwg", "cad"],
  ["cert.pem", "certificate"], ["message.eml", "mail"], ["event.ics", "calendar"],
  ["fix.patch", "diff"], ["flow.mmd", "diagram"], ["data.parquet", "dataset"],
  ["Dockerfile.dev", "container"], [".gitignore", "git"], ["Makefile", "script"],
  ["LICENSE", "text"], ["README", "text"], ["foo.unknown", "unknown"],
  ["report.pdf.exe", "binary"], ["folder.pdf/opaque", "unknown"],
])("classifies %s for display as %s", (path, kind) => {
  expect(fileAppearance(path).kind).toBe(kind);
});

it("distinguishes code languages and PDF with short labels", () => {
  expect(fileAppearance("main.ts").badge).toBe("TS");
  expect(fileAppearance("main.py").badge).toBe("PY");
  expect(fileAppearance("report.pdf").badge).toBe("PDF");
});
