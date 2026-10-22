import type { CapabilityScope, ScopeLayer } from "./mcp";

// Skills, which are prompts a project brought with it — named ones the user can
// call, and the rest the model decides about per task.
// One entry of GET /skills — every skill that may run, which is a larger set
// than /slash: a skill with no slash name still fires on model discovery.
export interface SkillEntry {
  name: string;
  slashName?: string;
  description?: string;
  scope?: string;
  plugin?: string;
  path?: string;
  // What uninstalling deletes, and whether that is a whole folder; absent when it cannot be removed.
  removes?: string;
  removesFolder?: boolean;
  subagent?: boolean;
  readOnly?: boolean;
  model?: string;
  effort?: string;
  allowedTools?: string[];
  // The specification's descriptive fields, as the skill wrote them.
  license?: string;
  compatibility?: string;
  metadata?: Record<string, string>;
  manual?: boolean;
  enabled: boolean;
  // Where the decision behind enabled lives, or absent when nothing overrides
  // what the skill itself declares.
  switchScope?: ScopeLayer;
  // Where an installed copy came from, and whether it still matches that folder.
  source?: SkillSource;
}

// What updating an installed copy from its source folder would change, as the
// person is shown it before agreeing.
export interface SkillUpdateChange {
  path: string;
  kind: "added" | "changed" | "removed";
  code: boolean;
  diff?: string;
  truncated?: boolean;
  binary?: boolean;
}

export interface SkillUpdatePlan {
  name: string;
  source: string;
  state: string;
  changes: SkillUpdateChange[];
  overwritesLocal: boolean;
  planId: string;
}

export interface SkillSource {
  path: string;
  mode: string;
  state: "synced" | "source_changed" | "local_edited" | "diverged" | "source_missing" | "unknown";
}

// implicit is the session-wide switch for model-initiated discovery. With it
// off every "auto" skill is manual in practice, so the rows have to say so.
export interface SkillCatalog {
  implicit: boolean;
  skills: SkillEntry[];
  scope?: CapabilityScope;
}
