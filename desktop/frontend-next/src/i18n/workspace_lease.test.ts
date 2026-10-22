// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { boot, STORAGE, t } from "./index";
import { NOTICE_TEXT } from "./notices";
import { say } from "./kernel";
import { workspaceLeaseDetail } from "./workspace_lease";

afterEach(() => { localStorage.setItem(STORAGE, "zh"); boot(); });

describe("write claim attribution", () => {
  for (const language of ["zh", "en"]) {
    it(`carries holder identity and extent in ${language}`, () => {
      localStorage.setItem(STORAGE, language);
      boot();
      const claim = { contended: 0, heldMs: 0, idleMs: 0, holder: "Fixture A", holderSessionId: "session-a", paths: ["src/a.go"], requestedPaths: ["src/b.go"] };
      const detail = workspaceLeaseDetail(claim);
      for (const value of ["Fixture A", "session-a", "src/a.go", "src/b.go"]) expect(detail).toContain(value);
      expect(workspaceLeaseDetail(claim, false)).not.toContain("Fixture A");
      expect(t(NOTICE_TEXT.workspace_lease_resumed)).toBe(language === "en"
        ? "This session has acquired its requested write claim and continued"
        : "这个会话已取得所需的写入范围，现已继续");
      expect(say({ code: "workspace.write_conflict" }, "")).not.toBe("");
    });
  }

  for (const language of ["zh", "en"]) {
    it(`says a repository folded the folders into one write domain, with the way out, in ${language}`, () => {
      localStorage.setItem(STORAGE, language);
      boot();
      const folded = { contended: 0, heldMs: 0, idleMs: 0, holder: "A", holderSessionId: "s", paths: ["d:/home/ws"], requestedPaths: ["d:/home/ws"], repository: "d:/home/ws" };
      const detail = workspaceLeaseDetail(folded);
      expect(detail).toContain("d:/home/ws");
      expect(detail).toContain(language === "en" ? "same Git repository" : "同一个 Git 仓库");
      expect(detail).toContain("write_lease");
      expect(workspaceLeaseDetail({ ...folded, repository: undefined })).not.toContain("write_lease");
      expect(workspaceLeaseDetail(folded, false)).not.toContain("write_lease");
    });
  }
});
