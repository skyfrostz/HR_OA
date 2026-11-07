import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { readdirSync, readFileSync } from "node:fs";
import type { Plugin } from "vite";

// Package only data assets from the PDF dependency already pinned by vue-pdf-embed.
// Fonts and CMaps are served locally; no CDN or document-controlled resource URL.
export function oaPdfResources(): Plugin {
  const require = createRequire(import.meta.url);
  const root = dirname(require.resolve("pdfjs-dist/package.json", {
    paths: [dirname(require.resolve("vue-pdf-embed"))]
  }));
  const resources = new Map<string, string>();
  for (const [source, target] of [["cmaps", "pdf-cmaps"], ["standard_fonts", "pdf-fonts"]]) {
    for (const name of readdirSync(join(root, source))) {
      if (/\.(bcmap|pfb|ttf)$/.test(name) || name.startsWith("LICENSE")) {
        resources.set("pure-assets/" + target + "/" + name, join(root, source, name));
      }
    }
  }
  return {
    name: "oa-local-pdf-resources",
    generateBundle() {
      for (const [fileName, path] of resources) this.emitFile({ type: "asset", fileName, source: readFileSync(path) });
    },
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const path = resources.get((req.url || "").split("?")[0].replace(/^\//, ""));
        if (!path) return next();
        res.setHeader("Content-Type", "application/octet-stream");
        res.end(readFileSync(path));
      });
    }
  };
}
