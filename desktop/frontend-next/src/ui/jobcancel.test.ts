import { expect, it, vi } from "vitest";
import { HttpError } from "../port/port";
import { cancelJob } from "./jobcancel";

it("reads the list again after a stop is answered", async () => {
  const refresh = vi.fn().mockResolvedValue(undefined);
  const notify = vi.fn();
  await cancelJob({ cancelJob: () => Promise.resolve() }, "job-1", notify, refresh);
  expect(refresh).toHaveBeenCalledTimes(1);
  expect(notify).not.toHaveBeenCalled();
});

it("tells the person and still reads the list again when the kernel refuses", async () => {
  const refusal = new HttpError(409, "gone", { code: "job.not_running" }, true);
  const refresh = vi.fn().mockResolvedValue(undefined);
  const notify = vi.fn();
  await cancelJob({ cancelJob: () => Promise.reject(refusal) }, "job-1", notify, refresh);
  expect(notify).toHaveBeenCalledWith(refusal);
  expect(refresh).toHaveBeenCalledTimes(1);
});
