// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { fromHistory } from "../state/session";
import { ToolCard } from "./cards/ToolCard";
import { openCards } from "./cards/openCards";

afterEach(cleanup);

describe("normalized tool arguments", () => {
  it("restores the effective input and original input without a failure", () => {
    const normalization = {
      reason: "redundant_arguments_envelope",
      original: '{"arguments":{"path":"test.txt"}}',
      effective: '{"path":"test.txt"}',
    };
    const { items } = fromHistory([
      { role: "assistant", content: "", toolCalls: [{
        id: "1", name: "read_file", arguments: normalization.effective,
        argumentNormalization: normalization,
      }] },
      { role: "tool", content: "contents", toolCallId: "1", toolName: "read_file" },
    ]);
    const item = items.find((item) => item.t === "tool");
    if (!item || item.t !== "tool") throw new Error("missing tool");
    expect(item.tool.args).toBe(normalization.effective);
    expect(item.tool.argumentNormalization).toEqual(normalization);
    expect(item.tool.err).toBeFalsy();
    const { container } = render(<ToolCard tool={item.tool} running={false} activity />);
    openCards(container);
    const html = container.innerHTML;
    expect(html).toContain("参数格式已自动兼容");
    expect(html).toContain("兼容前的原始输入");
    expect(html).toContain("arguments");
    expect(html).toContain("test.txt");
  });

  it("leaves legacy calls without a normalization notice", () => {
    const { container } = render(<ToolCard tool={{
      name: "read_file", args: '{"path":"a"}', readOnly: true, output: "contents",
    }} running={false} activity />);
    openCards(container);
    expect(container.innerHTML).not.toContain("参数格式已自动兼容");
  });
});
