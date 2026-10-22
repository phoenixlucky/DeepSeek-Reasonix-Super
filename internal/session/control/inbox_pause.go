package control

import "reasonix/internal/state/sessioninbox"

func setInboxPause(st *sessioninbox.Store, paused bool) error {
	if paused {
		return st.Pause(sessioninbox.PauseUser)
	}
	return st.Resume()
}

func (c *Controller) SetInboxPaused(paused bool) error {
	st, err := c.ensureInbox()
	if err != nil {
		return err
	}
	if err := setInboxPause(st, paused); err != nil {
		return err
	}
	if paused {
		sessioninbox.NotePaused()
	} else {
		c.maybeDispatchInbox()
	}
	return nil
}
