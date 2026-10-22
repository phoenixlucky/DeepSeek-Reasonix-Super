import { describe, expect, it } from "vitest";
import type { WorkspaceChange } from "../port/workspace";
import { buildTree, locate, stagedState } from "./gittree";

const c = (path: string, extra: Partial<WorkspaceChange> = {}): WorkspaceChange => ({ path, status: "M", unstaged: true, ...extra });

describe("the changes tree", () => {
  it("puts folders before files and sorts each by name", () => {
    const tree = buildTree([c("z.go"), c("b/y.go"), c("a/x.go"), c("a.go")]);
    expect(tree.map((n) => `${n.kind}:${n.name}`)).toEqual(["folder:a", "folder:b", "file:a.go", "file:z.go"]);
  });

  it("joins a folder that holds only another folder, as compact folders do, and stops where files begin", () => {
    const tree = buildTree([c("internal/provider/openai/chunk.go"), c("internal/provider/openai/stream.go"), c("internal/provider/retry.go")]);
    expect(tree).toHaveLength(1);
    const top = tree[0];
    expect(top.kind === "folder" && top.name).toBe("internal/provider");
    const kids = top.kind === "folder" ? top.children : [];
    expect(kids.map((n) => `${n.kind}:${n.name}`)).toEqual(["folder:openai", "file:retry.go"]);
  });

  it("rolls each folder up to the files under it, however deep, with counts", () => {
    const tree = buildTree([c("a/b/one.go", { insertions: 3, deletions: 1 }), c("a/two.go", { insertions: 4 }), c("a/b/bin.png")]);
    const folder = tree[0];
    expect(folder.kind).toBe("folder");
    if (folder.kind !== "folder") return;
    expect(folder.files.map((f) => f.path).sort()).toEqual(["a/b/bin.png", "a/b/one.go", "a/two.go"]);
    expect([folder.insertions, folder.deletions]).toEqual([7, 1]);
  });

  it("reports a folder wholly staged, partly staged or not staged", () => {
    expect(stagedState([c("a", { staged: true, unstaged: false }), c("b", { staged: true, unstaged: false })])).toBe(true);
    expect(stagedState([c("a", { staged: true, unstaged: false }), c("b")])).toBe("mixed");
    expect(stagedState([c("a", { staged: true, unstaged: true })])).toBe("mixed");
    expect(stagedState([c("a"), c("b")])).toBe(false);
    expect(stagedState([])).toBe(false);
  });

  it("is the same tree for the same list", () => {
    const list = [c("a/b.go"), c("a/c/d.go"), c("e.go")];
    expect(JSON.stringify(buildTree(list))).toBe(JSON.stringify(buildTree([...list])));
  });
});

describe("which repository a path belongs to", () => {
  const row = (dir: string) => ({ dir, name: dir || "ws", branch: "main", detached: false });
  it("gives a folder of projects' path to the deepest listed repository", () => {
    const repos = [row("api"), row("group/tools"), row("group")];
    expect(locate(repos, "api/main.go")).toEqual({ repo: "api", rel: "main.go" });
    expect(locate(repos, "group/tools/x/y.go")).toEqual({ repo: "group/tools", rel: "x/y.go" });
    expect(locate(repos, "group/other.go")).toEqual({ repo: "group", rel: "other.go" });
  });
  it("does not take a sibling whose name only starts the same", () => {
    expect(locate([row("api")], "api-extra/x.go")).toEqual({ repo: "", rel: "api-extra/x.go" });
  });
  it("gives every path to a workspace that is itself the repository", () => {
    expect(locate([row("")], "a/b.go")).toEqual({ repo: "", rel: "a/b.go" });
  });
});
