// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { entryScript, useStalePage } from "./stale_page";

const page = (src: string) => `<!doctype html><html><head><script type="module" crossorigin src="${src}"></script></head><body></body></html>`;

// The read the hook does is injected here rather than stubbed globally: a remote
// page's global fetch is replaced with a tunnel to the kernel, and the hook
// deliberately uses its own instead. Passing the stub in is what lets these
// cases read as the page's own entry.
function Probe({ enabled, fetchImpl }: { enabled: boolean; fetchImpl?: typeof fetch }) {
  return <div data-testid="stale">{useStalePage(enabled, fetchImpl) ? "stale" : "fresh"}</div>;
}

beforeEach(() => {
  document.head.innerHTML = '<script type="module" src="/assets/index-AAA.js"></script>';
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("entryScript", () => {
  it("reads the module entry, whatever attributes surround it", () => {
    expect(entryScript(page("/assets/index-BBB.js"))).toBe("/assets/index-BBB.js");
    expect(entryScript("<html>no script</html>")).toBeNull();
  });
});

describe("a page left open across a deploy", () => {
  const answer = (html: string | Error) =>
    vi.stubGlobal("fetch", vi.fn(async () => {
      if (html instanceof Error) throw html;
      return { ok: true, text: async () => html };
    }));

  it("says so once the hosted entry is a different build, on return to the tab", async () => {
    answer(page("/assets/index-BBB.js"));
    render(<Probe enabled fetchImpl={fetch} />);
    expect(screen.getByTestId("stale").textContent).toBe("fresh");
    document.dispatchEvent(new Event("visibilitychange"));
    await waitFor(() => expect(screen.getByTestId("stale").textContent).toBe("stale"));
  });

  it("stays quiet when the entry is the one running", async () => {
    answer(page("/assets/index-AAA.js"));
    render(<Probe enabled fetchImpl={fetch} />);
    document.dispatchEvent(new Event("visibilitychange"));
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(screen.getByTestId("stale").textContent).toBe("fresh");
  });

  it("does not guess from a failed or unreadable answer", async () => {
    answer(new Error("offline"));
    render(<Probe enabled fetchImpl={fetch} />);
    document.dispatchEvent(new Event("visibilitychange"));
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(screen.getByTestId("stale").textContent).toBe("fresh");
  });

  it("asks nothing when the page is not the hosted one", () => {
    answer(page("/assets/index-BBB.js"));
    render(<Probe enabled={false} fetchImpl={fetch} />);
    document.dispatchEvent(new Event("visibilitychange"));
    expect(fetch).not.toHaveBeenCalled();
  });
});

// A remote page reads the kernel's pages over the tunnel: globalThis.fetch has
// been replaced with one that forwards same-origin requests to the desktop. The
// hook must not go through it, or it compares this build against whatever the
// kernel is serving and reports an up-to-date page as behind forever.
describe("a remote page and the fetch the kernel replaced", () => {
  it("counts as up to date off its own entry, not the tunnelled one", async () => {
    // The running build is what the page's own origin answers with.
    const own = vi.fn(async () => ({ ok: true, text: async () => page("/assets/index-AAA.js") }));
    // What the tunnel would answer: the desktop's build, a different entry.
    const tunnelled = vi.fn(async () => ({ ok: true, text: async () => page("/assets/index-BBB.js") }));

    render(<Probe enabled fetchImpl={own as unknown as typeof fetch} />);
    vi.stubGlobal("fetch", tunnelled);

    document.dispatchEvent(new Event("visibilitychange"));
    await waitFor(() => expect(own).toHaveBeenCalled());
    expect(tunnelled).not.toHaveBeenCalled();
    expect(screen.getByTestId("stale").textContent).toBe("fresh");
  });
});
