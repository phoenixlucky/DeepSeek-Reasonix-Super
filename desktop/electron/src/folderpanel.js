"use strict";

// A person needs longer than this to read a panel and choose in it. A panel
// that closes sooner was never drawn: on Linux a missing file-chooser portal
// answers the same way as a dismissal, so the page would be told the person
// declined and would offer nothing else.
const UNDRAWN_MS = 400;

// folderAnswer is what the page is told about a folder panel: the path chosen,
// "" for a panel the person closed, and null for a panel that could not be shown,
// which the page answers by asking for the path in words.
function folderAnswer(result, elapsedMs, platform) {
  if (!result.canceled && result.filePaths.length > 0) return result.filePaths[0];
  return platform === "linux" && elapsedMs < UNDRAWN_MS ? null : "";
}

module.exports = { folderAnswer, UNDRAWN_MS };
