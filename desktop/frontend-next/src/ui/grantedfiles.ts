import type { AgentPort } from "../port/port";
import { docRef } from "./docrefs";
import { conversationFile } from "./filelinks";
import type { LocalRefs } from "./Markdown";

/** A file in a folder the user granted this workspace is carried as one path the
 *  workbench can key a tab on: this prefix, the folder, then the path inside it.
 *  The folder is spelled in the key so the key stays true if the grant list is
 *  reordered; the kernel decides what is readable by checking the folder against
 *  the grant list again, so a forged key reads nothing. */
export const GRANTED_FILE = ".granted-file";

export function grantedFilePath(root: string, rel: string): string {
  return `${GRANTED_FILE}/${encodeURIComponent(root)}/${rel}`;
}

export function parseGrantedFile(path: string): { root: string; rel: string } | null {
  const match = new RegExp(`^${GRANTED_FILE.replace(".", "\\.")}/([^/]+)/(.+)$`).exec(path);
  if (!match) return null;
  try {
    return { root: decodeURIComponent(match[1]), rel: match[2] };
  } catch {
    return null;
  }
}

/** The key of the file href names inside one of the granted folders, or null when
 *  it names none of them. Only a path spelled inside a folder counts: the same
 *  reading a picture gets, so what a file may be and what a picture may be agree. */
export function grantedFileKey(folders: string[], href: string): string | null {
  for (const folder of folders) {
    const rel = conversationFile(folder, href);
    if (rel && !rel.startsWith("../") && rel !== "..") return grantedFilePath(folder, rel);
  }
  return null;
}

/** What a rendered document's own links and pictures resolve to. Beside a file in a
 *  granted folder they stay inside that folder, so a relative link opens its sibling
 *  and a picture loads from the same folder; beside a workspace file, as before. */
export function documentRefs(filePath: string, port: AgentPort, open: (path: string) => void): LocalRefs {
  const held = parseGrantedFile(filePath);
  const resolve = (href: string) => docRef(held ? held.rel : filePath, href);
  return {
    link: (href) => {
      const to = resolve(href);
      return to ? () => open(held ? grantedFilePath(held.root, to) : to) : null;
    },
    image: (src) => {
      const at = resolve(src);
      return at ? port.workspaceImageURL(at, held?.root) : null;
    },
  };
}
