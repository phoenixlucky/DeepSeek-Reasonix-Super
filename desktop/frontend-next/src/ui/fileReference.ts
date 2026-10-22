/** Mirrors control.EscapeRefPath's @-token grammar, not JS or shell escaping.
 * Backslashes stay literal; only space/tab consume a preceding backslash.
 * Reject spellings whose token boundary, punctuation or MCP classification can
 * change the selected file when the person adds prose after the reference. */
export function fileReference(path: string): string {
  if (!path || path.includes(":") || path.endsWith("\\") || path.trimEnd() !== path || /[\r\n\f]|[.,;!?)}\]]$/.test(path)) return "";
  let token = "@";
  for (const character of path) {
    if (character === " " || character === "\t") token += "\\";
    token += character;
  }
  return token;
}
