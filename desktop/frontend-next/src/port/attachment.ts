// What a dropped or pasted thing becomes once the host has taken it: a token
// the turn parser resolves. The two shapes below differ only in what the client
// had to offer — bytes, or the path they came from.

export interface Attachment {
  path: string;
  ref: string;
  // Whether a text-only model will miss this one. The host decides, by the same
  // rule its resolver uses; restating it here is how the two drift.
  image?: boolean;
}

// One path a window reported under a drop. Only the host can answer: whether a
// path is inside the workspace compares two spellings of a location, and the
// token minted for anything outside must survive a grammar the page never sees.
export interface DroppedRef {
  ref?: string;
  path?: string;
  image?: boolean;
  error?: string;
}

// A paste a person chose to keep out of the conversation. pointer is the text
// that stands in for it in the message: size, where it is, how to read it. There
// is no "@" reference on purpose, because the turn parser expands one into the
// file's head and would put the text back.
export interface KeptText {
  path: string;
  pointer: string;
}
