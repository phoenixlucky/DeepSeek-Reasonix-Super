import { afterEach, describe, expect, it } from "vitest";
import { clearToasts, dismissToast, MAX_TOASTS, pushToast, readToasts } from "./toasts";

afterEach(clearToasts);

const texts = () => readToasts().map((toast) => toast.text);

describe("the corner notices", () => {
  it("keeps what it was told until it is dismissed, and says each thing once", () => {
    pushToast("model switch failed");
    pushToast("model switch failed");
    pushToast("branch switch failed");
    expect(texts()).toEqual(["model switch failed", "branch switch failed"]);
    dismissToast(readToasts()[0].id);
    expect(texts()).toEqual(["branch switch failed"]);
  });

  it("never grows past the corner's ceiling, dropping the oldest", () => {
    for (let i = 0; i < MAX_TOASTS + 2; i++) pushToast(`failure ${i}`);
    expect(texts()).toEqual(["failure 2", "failure 3", "failure 4"]);
  });

  it("ignores an empty message", () => {
    pushToast("   ");
    expect(texts()).toEqual([]);
  });
});
