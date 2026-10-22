"use strict";

// Who owns the keyboard while the agent drives a page. A person typing in the
// app page keeps typing there: a page the agent opens, navigates or puts on
// screen is given the keyboard by the platform, and nothing the agent does is a
// reason for it to have it. The one reason that counts is the person's own: a
// click or a key that reached the page from the window, which is theirs to
// have chosen. The agent's own input arrives over the debugging protocol and
// is recognised as such, because it reaches the page the same way a click does.

const PERSON_MS = 400;

class KeyboardKeeper {
  constructor({ win, now = Date.now }) {
    this.win = win;
    this.now = now;
    this.owner = win.webContents.isFocused() ? "app" : "none";
    win.webContents.on("focus", () => {
      this.owner = "app";
    });
  }

  // watch follows one page. synthetic() says whether input reaching it right now
  // is the agent's own.
  watch(contents, synthetic = () => false) {
    let personAt = -Infinity;
    contents.on("before-input-event", () => {
      personAt = this.now();
    });
    contents.on("input-event", (_event, input) => {
      if (input && input.type === "mouseDown" && !synthetic()) personAt = this.now();
    });
    contents.on("focus", () => {
      if (this.now() - personAt < PERSON_MS) {
        this.owner = "view";
        return;
      }
      if (this.owner !== "app" || !this.live()) {
        this.owner = "view";
        return;
      }
      // The platform is still handing the page the keyboard while this event is
      // delivered, and a focus asked for from inside it is lost; one a turn later
      // is not, as long as nothing of the person's has arrived by then.
      setTimeout(() => {
        if (this.now() - personAt < PERSON_MS || this.owner !== "app" || !this.live()) return;
        if (contents.isFocused()) this.win.webContents.focus();
      }, 0);
    });
  }

  // reclaim gives the keyboard back to the app page when attaching or moving a
  // view left nobody holding it, while the window itself is the one in front.
  reclaim() {
    if (this.owner === "app" && this.live() && !this.win.webContents.isFocused()) this.win.webContents.focus();
  }

  live() {
    return !this.win.isDestroyed() && this.win.isFocused();
  }
}

module.exports = { KeyboardKeeper, PERSON_MS };
