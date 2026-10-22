package cli

import (
	"fmt"
	"path/filepath"
	"reasonix/internal/state/sessionstore"
	"strings"

	"reasonix/internal/session/control"
)

// sessionLeaseResumeRefusal is the startup-time refusal for `reasonix
// [--resume|--continue]` and `reasonix run --resume/--continue`: it names the
// holder and offers the two ways out (close the holder, or continue in a
// duplicated session via --copy).
func sessionLeaseResumeRefusal(err error, takeover bool) string {
	if takeover {
		return control.SessionInUseMessage(err) +
			"; --takeover cannot take a session from another Reasonix process: close it, or rerun with --copy to continue in a duplicated session"
	}
	return control.SessionInUseMessage(err) +
		"; close the other Reasonix window or process, or rerun with --copy to continue in a duplicated session"
}

// cliSessionRecoveredHandler moves the single-session CLI lease during the
// controller's recovery commit. The callback runs before Controller changes its
// session path, closing the unguarded interval that event-driven follow-up
// calls left after ordinary turn-end and mid-turn autosaves.
func cliSessionRecoveredHandler(leases *control.SessionLeaseKeeper) func(control.SessionRecoveryInfo) error {
	return func(info control.SessionRecoveryInfo) error {
		if err := leases.HandleSessionRecovered(info); err != nil {
			return err
		}
		// Controller pointer is not available here; TUI followSessionLease and
		// headless post-Rebind bind authority. Recovery commit rebinds the lease
		// first; the next Snapshot path match is ensured once Bind runs.
		return nil
	}
}

// copySessionForWriting duplicates the session at src into a fresh session
// file beside it and returns the new path. It backs the --copy escape hatch:
// when src is held by another runtime, the copy gives this process a session
// it can own. The duplicate is written through Session.SaveIfAbsent, so it is
// event-log aware (authoritative event log plus .jsonl checkpoint), cannot
// replace a destination another runtime created, and starts with no
// lease/lock sidecars of its own; src is only read. When src is being
// written concurrently, the copy captures the transcript as of the load — an
// append-only prefix, the same view a resume would see.
func copySessionForWriting(src string) (string, error) {
	loaded, err := loadResumableSession(src)
	if err != nil {
		return "", err
	}
	msgs := loaded.Snapshot()

	var srcMeta sessionstore.BranchMeta
	if meta, ok, metaErr := sessionstore.LoadBranchMeta(src); metaErr == nil && ok {
		srcMeta = meta
	}
	label := "session"
	if model, ok := sessionstore.LoadSessionModel(src); ok && strings.TrimSpace(model) != "" {
		label = model
	}

	newPath := sessionstore.NewSessionPath(filepath.Dir(src), label)
	copySess := sessionstore.NewSession("")
	copySess.Messages = msgs
	if err := copySess.SaveIfAbsent(newPath); err != nil {
		return "", fmt.Errorf("copy session: %w", err)
	}
	preview, turns := sessionstore.SessionPreviewFromMessages(msgs)
	meta := sessionstore.BranchMeta{
		ParentID:      sessionstore.BranchID(src),
		Preview:       preview,
		Turns:         turns,
		SchemaVersion: sessionstore.BranchMetaCountsVersion,
		Model:         srcMeta.Model,
	}
	if title := strings.TrimSpace(firstNonEmpty(srcMeta.CustomTitle, srcMeta.TopicTitle)); title != "" {
		meta.CustomTitle = title + " (copy)"
	}
	if err := sessionstore.SaveBranchMeta(newPath, meta); err != nil {
		return "", fmt.Errorf("copy session meta: %w", err)
	}
	return newPath, nil
}
