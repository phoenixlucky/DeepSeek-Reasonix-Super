import { docRef } from "./docrefs";

export function conversationFile(root: string, href: string): string | null {
  let raw = href.trim();
  if (/^file:/i.test(raw)) {
    try {
      const url = new URL(raw);
      if (url.hostname && url.hostname !== "localhost") return null;
      raw = url.pathname;
      if (/^\/[a-z]:\//i.test(raw)) raw = raw.slice(1);
    } catch { return null; }
  } else if (/^[a-z][a-z0-9+.-]*:/i.test(raw) && !/^[a-z]:[\\/]/i.test(raw)) return null;
  const windows = /^[a-z]:[\\/]/i.test(root);
  const base = (windows ? root.replaceAll("\\", "/") : root).replace(/\/+$/, "");
  if (windows) raw = raw.replaceAll("\\", "/");
  raw = raw.replace(/[?#].*$/, "");
  try { raw = decodeURIComponent(raw); } catch { return null; }
  if (raw.startsWith("//")) return null;
  if (raw.startsWith("/") || /^[a-z]:[\\/]/i.test(raw)) {
    const prefix = base + "/";
    if (!(windows ? raw.toLowerCase().startsWith(prefix.toLowerCase()) : raw.startsWith(prefix))) return null;
    raw = raw.slice(prefix.length);
  }
  // docRef decodes URLs; encode literal filename characters before resolving.
  return docRef("", raw.split("/").map(encodeURIComponent).join("/"));
}

/** Whether src is spelled as a file on this machine: a file: URL, a drive path,
 *  a rooted path or a UNC share. The shape decides; what it names never does. */
/** What an image source becomes when it is spelled as a local file the workspace does not hold. */
export const OUTSIDE_IMAGE = "#outside-image";

export function isLocalPathShape(src: string): boolean {
  const raw = src.trim();
  return /^file:/i.test(raw) || /^[a-z]:[\\/]/i.test(raw) || (raw.startsWith("/") && !raw.startsWith("//")) || raw.startsWith("\\\\");
}

interface Node {
  type: string;
  url?: string;
  value?: string;
  children?: Node[];
}

const TOKEN = /[^\s<>"'`()\[\]{}，。；！？：]+/g;
const FILE = /(?:^|\/)[^/]+\.[a-z0-9]{1,16}$/i;

export function remarkFileLinks(resolve?: (href: string) => string | null) {
  return (tree: Node) => {
    if (!resolve) return;
    const walk = (node: Node) => {
      if (node.type === "link") {
        const path = node.url && resolve(node.url);
        if (path) node.url = path.split("/").map(encodeURIComponent).join("/");
        return;
      }
      if (!node.children) return;
      node.children = node.children.flatMap((child) => {
        if (child.type === "inlineCode" && child.value && /[\\/]/.test(child.value) && FILE.test(child.value.replaceAll("\\", "/"))) {
          const path = resolve(child.value);
          if (path) return [{ type: "link", url: path.split("/").map(encodeURIComponent).join("/"), children: [child] }];
        }
        if (child.type === "text" && child.value) {
          const out: Node[] = [];
          let at = 0;
          for (const match of child.value.matchAll(TOKEN)) {
            const value = match[0].replace(/[.,;!?]+$/, "");
            if (!/[\\/]/.test(value) || !FILE.test(value.replaceAll("\\", "/"))) continue;
            const path = resolve(value);
            if (!path) continue;
            if (match.index > at) out.push({ type: "text", value: child.value.slice(at, match.index) });
            out.push({ type: "link", url: path.split("/").map(encodeURIComponent).join("/"), children: [{ type: "text", value }] });
            at = match.index + value.length;
          }
          if (out.length) {
            if (at < child.value.length) out.push({ type: "text", value: child.value.slice(at) });
            return out;
          }
        }
        walk(child);
        return [child];
      });
    };
    walk(tree);
  };
}

/** A picture in a folder the user granted is carried as GRANTED/<n>/<path> until the
 *  image is loaded, n being the folder's place in the granted list. */
export const GRANTED = ".granted";

/** A picture the model wrote as a file path: inside the workspace it becomes a
 *  workspace-relative address the controlled image channel can serve, outside it
 *  becomes OUTSIDE_IMAGE, so no local address ever reaches an <img>. */
export function remarkLocalImages(resolve?: (src: string) => string | null, granted?: (src: string) => string | null) {
  return (tree: Node) => {
    if (!resolve) return;
    const walk = (node: Node) => {
      if (node.type === "image" && node.url) {
        const path = resolve(node.url) ?? (granted && isLocalPathShape(node.url) ? granted(node.url) : null);
        if (path) node.url = path.split("/").map(encodeURIComponent).join("/");
        else if (isLocalPathShape(node.url)) node.url = OUTSIDE_IMAGE;
      }
      node.children?.forEach(walk);
    };
    walk(tree);
  };
}
