// @vitest-environment jsdom
import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useSubmit } from "./usesubmit";
import { HttpError, type AgentPort } from "../port/port";

const hook = (submit: () => Promise<unknown>) => {
  const dispatch = vi.fn();
  const fail = vi.fn();
  const port = { submit, queueFollowup: vi.fn(), steer: vi.fn() } as unknown as AgentPort;
  const { result } = renderHook(() => useSubmit({ port, running: false, dispatch, trajDispatch: vi.fn(), refreshStatus: vi.fn(), fail }));
  return { send: result.current, dispatch, fail };
};

describe("useSubmit", () => {
  it("says a `!` command was refused for this connection and takes the row back", async () => {
    const { send, dispatch, fail } = hook(async () => { throw new HttpError(403, "no shell", { code: "shell.unavailable_over_http" }); });
    expect(await send("!ls")).toBe("shell_unavailable");
    expect(dispatch.mock.calls.some(([a]) => (a as { kind: string }).kind === "__unsent")).toBe(true);
    expect(fail).toHaveBeenCalledTimes(1);
  });

  it("keeps any other refusal a plain not-sent", async () => {
    const { send } = hook(async () => { throw new HttpError(409, "taken", { code: "provider.name_taken" }); });
    expect(await send("hello")).toBe(false);
  });

  it("answers true once the kernel took the line", async () => {
    const { send } = hook(async () => undefined);
    expect(await send("hello")).toBe(true);
  });
});
