import type { Item } from "../../state/session";
import type { DeliveredFile } from "../../port/workspace";
import { opensTurn } from "../blocks";

export function deliveriesByReply(items: Item[]): Map<string, DeliveredFile[]> {
  const out = new Map<string, DeliveredFile[]>();
  let files = new Map<string, DeliveredFile>();
  let anchor = "", call = "";
  const flush = () => {
    if (files.size && (anchor || call)) out.set(anchor || call, [...files.values()]);
    files = new Map(); anchor = ""; call = "";
  };
  for (const item of items) {
    if (opensTurn(item)) flush();
    if (item.t === "say" && item.text) anchor = item.id;
    if (item.t === "tool" && !item.running && !item.tool.err && item.tool.deliveries?.length) {
      call = item.id;
      for (const file of item.tool.deliveries) files.set(JSON.stringify([file.root ?? "", file.path]), file);
    }
  }
  flush();
  return out;
}
