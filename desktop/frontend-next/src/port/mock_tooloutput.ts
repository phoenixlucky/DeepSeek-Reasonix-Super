import { HttpError } from "./http_error";
import type { ToolOutputPage } from "./session";
import { MockGit } from "./mock_git";

// The windows onto a call's whole output that a page asks for when it kept only
// the ends of it.
export class MockToolOutput extends MockGit {
  // The full output a call produced, by call id, for the windows toolOutput serves.
  fullOutputs: Record<string, string> = {};

  async toolOutput(callId: string, offset = 0, limit = 64 * 1024): Promise<ToolOutputPage> {
    const text = this.fullOutputs[callId];
    if (text === undefined) throw new HttpError(404, "no tool call with this id in this session", { code: "tool.output_unknown" });
    const bytes = new TextEncoder().encode(text);
    const from = Math.min(offset, bytes.length);
    const to = Math.min(bytes.length, from + limit);
    return { callId, text: new TextDecoder().decode(bytes.slice(from, to)), bytes: bytes.length, offset: from, truncated: to < bytes.length };
  }
}
