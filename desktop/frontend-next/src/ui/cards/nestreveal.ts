const nests = new Map<string, () => void>();

// A finished delegate's child calls are not mounted until its card is open. A
// jump to one of them (the run graph) asks here for the card that holds it to
// open, since there is no node to scroll to before that.
export function registerNest(ids: string[], open: () => void): () => void {
  for (const id of ids) nests.set(id, open);
  return () => {
    for (const id of ids) if (nests.get(id) === open) nests.delete(id);
  };
}

export function revealNested(id: string): void {
  nests.get(id)?.();
}
