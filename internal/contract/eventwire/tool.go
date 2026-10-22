package eventwire

import (
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

func toWireTool(t event.Tool) *Tool {
	wt := &Tool{
		ID: t.ID, Name: t.Name, Args: SurfaceArgs(t.Name, t.ResolvedName, t.Args),
		ArgumentNormalization: SurfaceArgumentNormalization(t.Name, t.ResolvedName, t.ArgumentNormalization),
		ResolvedName:          t.ResolvedName, CapabilityID: t.CapabilityID,
		Deliveries: t.Deliveries,
		Output:     t.Output, Images: t.Images, Videos: toWireVideos(t.Videos), Err: t.Err, RefusalCode: t.RefusalCode,
		WorkspaceLease: toWireWorkspaceLease(t.WorkspaceLease),
		OutputDiff:     t.OutputDiff,
		ReadOnly:       t.ReadOnly, Truncated: t.Bound.Lossy(),
		DurationMs: t.DurationMs, RateTokensPerSec: t.RateTokensPerSec, ContextTokens: t.ContextTokens(),
		Partial: t.Partial, StartedAt: t.StartedAt, EndedAt: t.EndedAt,
		ArgChars: t.ArgChars, Refreshed: t.Refreshed,
		ParentID: t.ParentID, AttemptID: t.AttemptID, Issuer: string(t.Issuer),
		Diff: t.Diff, Added: t.Added, Removed: t.Removed,
	}
	if b := t.Bound; b.Kind != event.BoundWhole {
		wt.Bound = &Bound{
			Kind: boundKindNames[b.Kind], Lines: b.Lines,
			Bytes: b.Bytes, KeptBytes: b.KeptBytes, Path: b.Path,
		}
	}
	if t.Profile != nil {
		wt.Profile = &Profile{Name: t.Profile.Name, Count: t.Profile.Count, Model: t.Profile.Model, Effort: t.Profile.Effort, Label: t.Profile.Label}
	}
	if t.Execution != nil {
		wt.Execution = toWireShellExecution(t.Execution)
	}
	return wt
}

// SurfaceArgumentNormalization applies the same redaction as displayed arguments
// without mutating canonical session metadata.
func SurfaceArgumentNormalization(name, resolved string, n *provider.ArgumentNormalization) *provider.ArgumentNormalization {
	if n == nil {
		return nil
	}
	out := *n
	out.Original = SurfaceArgs(name, resolved, out.Original)
	out.Effective = SurfaceArgs(name, resolved, out.Effective)
	return &out
}

// toWireWorkspace renders a workspace mutation. A nil payload still renders,
// so a frontend reading the field never has to tell "no changes" apart from
// "the event arrived without one".
func toWireWorkspace(ws *event.WorkspaceChangedPayload) *WorkspaceChanged {
	if ws == nil {
		ws = &event.WorkspaceChangedPayload{}
	}
	changes := make([]WorkspacePathChange, 0, len(ws.Changes))
	for _, c := range ws.Changes {
		changes = append(changes, WorkspacePathChange{Path: c.Path, OldPath: c.OldPath, Op: c.Op})
	}
	return &WorkspaceChanged{
		Revisions: WorkspaceRevision{Content: ws.Revisions.Content, Tree: ws.Revisions.Tree, WorkingTree: ws.Revisions.WorkingTree, GitMeta: ws.Revisions.GitMeta, Session: ws.Revisions.Session},
		Changes:   changes, AllPaths: ws.AllPaths, Source: ws.Source, WatchState: string(ws.WatchState),
	}
}
