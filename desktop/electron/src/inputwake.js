"use strict";

// A page in a minimized Windows window has no surface to hit-test, so the mouse
// input the agent sends it is dropped while the call still answers and typed
// text still lands. Input is therefore sent with the window raised without
// activation, fully transparent and deaf to the mouse, and the window is
// minimized again once the agent has been quiet for a moment.

const SETTLE_MS = 300;
const HOLD_MS = 500;

class InputWake {
  constructor({ win, platform = process.platform, settleMs = SETTLE_MS, holdMs = HOLD_MS }) {
    this.win = win;
    this.enabled = platform === "win32";
    this.settleMs = settleMs;
    this.holdMs = holdMs;
    this.raised = null;
    this.running = 0;
    this.release = this.release.bind(this);
  }

  // run performs one input call with the window able to take it.
  async run(call) {
    this.running++;
    clearTimeout(this.timer);
    try {
      await this.raise();
      return await call();
    } finally {
      if (--this.running === 0) this.timer = setTimeout(this.release, this.holdMs);
    }
  }

  raise() {
    const win = this.win;
    if (!this.enabled || win.isDestroyed()) return Promise.resolve();
    if (this.raised) return this.raised;
    if (!win.isMinimized()) return Promise.resolve();
    this.raised = new Promise((resolve) => setTimeout(resolve, this.settleMs));
    try {
      win.setOpacity(0);
      win.setIgnoreMouseEvents(true);
      win.showInactive();
    } catch (err) {
      this.release();
      throw err;
    }
    return this.raised;
  }

  release() {
    if (!this.raised) return;
    this.raised = null;
    const win = this.win;
    if (win.isDestroyed()) return;
    const theirs = win.isFocused();
    win.setIgnoreMouseEvents(false);
    win.setOpacity(1);
    if (!theirs) win.minimize();
  }
}

module.exports = { InputWake, SETTLE_MS, HOLD_MS };
