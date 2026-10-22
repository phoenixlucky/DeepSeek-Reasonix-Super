import { HttpError } from "./http_error";
import type { ChangeDiff, DiscardRequest, GitCommitFiles, GitRepoRow, GitRepos, PushPlan, PushRequest, PushResult, WorkspaceChange } from "./workspace";
import { MockFeedback } from "./mock_feedback";

const refuse = (status: number, code: string, error: string) => new HttpError(status, error, { code, error });

// The git panel's reads and writes beyond the workspace's own changes: which
// repositories there are, what a commit holds, a discard and a push. Scripted,
// because there is no repository behind the fixture, but held in fields so a
// write is seen by the next read the way the kernel's would be.
export class MockGit extends MockFeedback {
  // One entry is the workspace being a repository; several are a folder of projects.
  repoRows: GitRepoRow[] = [{ dir: "", name: "reasonix", branch: "main", detached: false }];

  // The changes of each repository below the workspace; the workspace's own are MockWorkspace.changeList.
  repoChanges: Record<string, WorkspaceChange[]> = {};

  async gitRepos(): Promise<GitRepos> {
    return { repos: this.repoRows.map((r) => ({ ...r })), truncated: false };
  }

  async discardChange(req: DiscardRequest): Promise<void> {
    const list = req.repo ? this.repoChanges[req.repo] : (this as unknown as { changeList: WorkspaceChange[] }).changeList;
    const found = list?.find((c) => c.path === req.path);
    if (!found) throw refuse(400, "discard.unknown_path", "not a changed file in the workspace");
    if (!["M", "D", "MM"].includes(found.status)) throw refuse(409, "discard.not_discardable", "only a change to a file the last commit holds can be discarded");
    const rest = list.filter((c) => c !== found);
    if (req.repo) this.repoChanges[req.repo] = rest;
    else (this as unknown as { changeList: WorkspaceChange[] }).changeList = rest;
  }

  async commitFiles(id: string): Promise<GitCommitFiles> {
    if (!/^[0-9a-f]{1,64}$/i.test(id)) throw refuse(400, "git.commit_bad_id", "that is not a commit id");
    return {
      repo: true,
      truncated: false,
      files: [
        { path: "internal/provider/retry.go", status: "M", insertions: 14, deletions: 3 },
        { path: "internal/provider/retry_test.go", status: "A", insertions: 42 },
        { path: "docs/old-notes.md", oldPath: "docs/notes.md", status: "R", insertions: 1, deletions: 1 },
        { path: "assets/logo.png", status: "M" },
      ],
    };
  }

  async commitDiff(_id: string, path: string): Promise<ChangeDiff> {
    return {
      path,
      truncated: false,
      diff: [`--- a/${path}`, `+++ b/${path}`, "@@ -1,3 +1,4 @@", " package provider", "-func retry() {}", "+func retry(after time.Duration) {", "+	time.Sleep(after)", " }"].join("\n"),
    };
  }

  // Commits the branch has that the upstream does not. Pushing sets it to zero.
  pushAhead = 2;
  pushScript: "ok" | "rejected" | "auth" | "untrusted" | "protocol" = "ok";

  private plan(repo?: string): PushPlan {
    return {
      remote: "origin", branch: "main", upstreamBranch: "main", scheme: "https", host: "github.com",
      url: "https://github.com/esengine/DeepSeek-Reasonix.git", head: repo ? "9f2c41b07ad3" : "0123456789ab", ahead: this.pushAhead, behind: 0,
    };
  }

  async pushPreview(repo?: string): Promise<PushPlan> {
    if (this.pushScript === "untrusted") throw refuse(403, "push.workspace_untrusted", "this workspace is not trusted");
    if (this.pushScript === "protocol") throw refuse(403, "push.protocol_not_allowed", "the remote's protocol is not allowed");
    return this.plan(repo);
  }

  async push(req: PushRequest): Promise<PushResult> {
    await new Promise((r) => setTimeout(r, 400));
    if (this.pushAhead === 0) throw refuse(409, "push.nothing_to_push", "nothing to push");
    if (this.pushScript === "rejected") throw refuse(409, "push.rejected", "the remote has commits this branch does not");
    if (this.pushScript === "auth") throw refuse(409, "push.auth_required", "the remote needs authentication");
    this.pushAhead = 0;
    return { plan: req.plan, new: false };
  }
}
