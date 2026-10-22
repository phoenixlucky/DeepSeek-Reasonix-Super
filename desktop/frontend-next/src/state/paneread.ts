import type { RuntimeView } from "../port/hub";

/**
 * Reads the pane list so that a failed read is never mistaken for an empty one.
 * Once a read has answered, a later failure returns null and the caller keeps
 * what it holds; before that, a failure reads as empty so a window never stays
 * blank waiting for a list that is not coming.
 */
export function paneReader(read: () => Promise<RuntimeView[]>): () => Promise<RuntimeView[] | null> {
  let answered = false;
  return async () => {
    try {
      const list = await read();
      answered = true;
      return list;
    } catch {
      return answered ? null : [];
    }
  };
}
