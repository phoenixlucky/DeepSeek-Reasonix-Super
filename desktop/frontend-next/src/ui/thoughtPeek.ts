const segmenter = new Intl.Segmenter(undefined, { granularity: "sentence" });

/** The first sentence of a thought, cut to `max` characters with an ellipsis. */
export function firstSentence(reasoning: string, max: number): string {
  const text = reasoning.trim();
  if (!text) return "";
  const first = segmenter.segment(text)[Symbol.iterator]().next().value?.segment.trim() ?? "";
  const chars = [...first.replace(/\s+/g, " ")];
  return chars.length > max ? `${chars.slice(0, max).join("")}…` : chars.join("");
}
