"use strict";

// The kernel's gate answers every request without the launch credential with
// 403, and a cookie belongs to the host, not the port, so anything that removes it (an
// eviction, a page clearing cookies for 127.0.0.1) leaves the window refused
// until a relaunch. The shell holds the token, so it puts the cookie back.
function keepCredential(ses, cookie, log) {
  const host = new URL(cookie.url).hostname;
  const restore = (cause) => {
    log(`credential cookie removed (${cause}); restoring`);
    ses.cookies.set(cookie).catch((err) => log(`credential cookie restore failed: ${err.message}`));
  };
  const onChanged = (_event, changed, cause, removed) => {
    if (!removed || changed.name !== cookie.name || cause === "overwrite") return;
    if (changed.domain.replace(/^\./, "") !== host) return;
    restore(cause);
  };
  ses.cookies.on("changed", onChanged);
  return () => ses.cookies.removeListener("changed", onChanged);
}

module.exports = { keepCredential };
