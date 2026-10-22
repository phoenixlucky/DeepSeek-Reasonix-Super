package sessioninbox

import "time"

// heldDuplicateLocked finds the still-waiting copy of an instruction (the
// checksum covers only what was asked). The same words from the same person
// before the first copy ran are one instruction: a second item would be a second
// turn. An unapplied steer counts as the first copy, and its words are its retry.
// Runtime notes never come here; their idempotency key tells them apart.
func (s *Store) heldDuplicateLocked(checksum string, origin PromptOrigin) (InboxReceipt, bool, error) {
	if checksum == "" {
		return InboxReceipt{}, false, nil
	}
	for _, item := range s.man.Items {
		if item.Checksum != checksum || item.Origin != origin {
			continue
		}
		switch {
		case item.State == StateQueued:
		case item.State == StateUncertain && item.BlockCode == BlockSteerUnapplied:
			next := s.man.clone()
			i := next.indexOf(item.ID)
			next.Items[i].State = StateQueued
			next.Items[i].Intent = IntentFollowup
			next.Items[i].BlockReason, next.Items[i].BlockCode = "", ""
			next.Items[i].UpdatedAt = time.Now().UTC()
			if err := s.commitManifestLocked(next); err != nil {
				return InboxReceipt{}, false, err
			}
			s.notifyLocked(s.snapshotLocked())
		default:
			continue
		}
		return InboxReceipt{
			ItemID: item.ID, Disposition: DispositionIdempotentHit,
			Position: s.man.indexOf(item.ID) + 1, Paused: s.man.Paused,
			Capacity: s.snapshotLocked().Capacity, Idempotent: true,
		}, true, nil
	}
	return InboxReceipt{}, false, nil
}
