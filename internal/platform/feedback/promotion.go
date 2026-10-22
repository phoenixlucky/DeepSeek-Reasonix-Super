package feedback

// noteLevel reads the level the service states against what this machine has
// announced. The first level ever seen is the baseline, so a reinstall or a
// second computer says nothing; a higher one is a promotion, to the highest
// level however many it jumped; a revoked standing is never announced.
func (st *state) noteLevel(p *Profile) *Promotion {
	if p == nil || p.TrustState == TrustRevoked {
		return nil
	}
	if st.AnnouncedLevel == nil {
		level := p.Level
		st.AnnouncedLevel = &level
		return nil
	}
	if p.Level > *st.AnnouncedLevel {
		return &Promotion{From: *st.AnnouncedLevel, To: p.Level}
	}
	return nil
}

// AckPromotion records that the person has been told about level. It only ever
// raises the mark, and nothing else moves it: reading, reloading or replaying a
// list never does.
func (s *Service) AckPromotion(level int) error {
	return s.store.update(func(st *state) error {
		if st.AnnouncedLevel == nil || level > *st.AnnouncedLevel {
			st.AnnouncedLevel = &level
		}
		return nil
	})
}
