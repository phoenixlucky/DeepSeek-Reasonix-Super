import { parseDelimited } from "./parseDelimited";

self.onmessage = (event: MessageEvent<{ text: string; delimiter: string }>) => {
  try {
    self.postMessage({ rows: parseDelimited(event.data.text, event.data.delimiter) });
  } catch {
    self.postMessage({ error: true });
  }
};
