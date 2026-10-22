import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { createRequire } from "node:module";
import type { Plugin } from "vite";

export function pdfAssets(): Plugin {
  const require = createRequire(import.meta.url);
  const root = dirname(require.resolve("pdfjs-dist/package.json"));
  const files = new Map<string, string>();
  for (const dir of ["cmaps", "standard_fonts", "wasm"]) {
    for (const name of readdirSync(join(root, dir))) files.set(`pdfjs/${dir}/${name}`, join(root, dir, name));
  }
  files.set("pdfjs/LICENSE", join(root, "LICENSE"));
  files.set("licenses/papaparse.txt", join(dirname(require.resolve("papaparse")), "LICENSE"));
  const purify = dirname(dirname(require.resolve("dompurify")));
  files.set("licenses/dompurify.txt", join(purify, "LICENSE"));
  files.set("licenses/dompurify-mpl.txt", join(purify, "LICENSE-MPL"));
  files.set("licenses/lucide-react.txt", join(dirname(require.resolve("lucide-react/package.json")), "LICENSE"));
  return {
    name: "pdf-local-assets",
    generateBundle() {
      for (const [fileName, path] of files) this.emitFile({ type: "asset", fileName, source: readFileSync(path) });
    },
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const path = files.get((req.url ?? "").split("?")[0].replace(/^\//, ""));
        if (!path) return next();
        res.setHeader("Content-Type", path.endsWith(".wasm") ? "application/wasm" : "application/octet-stream");
        res.end(readFileSync(path));
      });
    },
  };
}
