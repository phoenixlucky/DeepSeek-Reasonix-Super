package feedback

import (
	"context"
	"errors"
	"strings"
)

// Withdraw takes one of this install's reports back. The service alone decides
// whether it still can: a report that already has an outcome is
// ErrNotWithdrawable. What this machine remembers is updated only after the
// service said yes, so the list never shows a withdrawal that did not happen.
func (s *Service) Withdraw(ctx context.Context, receipt string) error {
	receipt = strings.TrimSpace(receipt)
	if !receiptPattern.MatchString(receipt) {
		return invalid(FieldReceipt, ReasonBadValue)
	}
	st, err := s.store.load()
	if err != nil {
		return err
	}
	it, ok := st.find(receipt)
	switch {
	case !ok:
		return invalid(FieldReceipt, ReasonBadValue)
	case it.StatusUnavailable || st.InstallID == "" || st.InstallToken == "":
		return ErrNotWithdrawable
	}
	err = s.postWithdraw(ctx, st.InstallID, st.InstallToken, it.Receipt)
	if errors.Is(err, ErrBadToken) {
		if rerr := s.store.update(func(st *state) error { st.retire(); return nil }); rerr != nil {
			return rerr
		}
	}
	if err != nil {
		return err
	}
	mine := it.Receipt
	return s.store.update(func(st *state) error {
		for i := range st.Items {
			if st.Items[i].Receipt == mine {
				st.Items[i].Status = StatusWithdrawn
				st.Items[i].NeedsInput = false
				st.Items[i].UpdatedAt = now().UTC()
			}
		}
		return nil
	})
}
