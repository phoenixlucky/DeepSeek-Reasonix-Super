package memory

import (
	"testing"
	"time"
)

type checkedMemoryStore struct {
	Store
	t *testing.T
}

func mustStore(t *testing.T, store Store) checkedMemoryStore {
	t.Helper()
	return checkedMemoryStore{Store: store, t: t}
}

func (s checkedMemoryStore) check(err error) {
	s.t.Helper()
	if err != nil {
		s.t.Fatal(err)
	}
}

func (s checkedMemoryStore) Read(ref string) (Memory, bool) {
	s.t.Helper()
	m, ok, err := s.Store.Read(ref)
	s.check(err)
	return m, ok
}

func (s checkedMemoryStore) Path(ref string) string {
	s.t.Helper()
	path, err := s.Store.Path(ref)
	s.check(err)
	return path
}

func (s checkedMemoryStore) List() []Memory {
	s.t.Helper()
	list, err := s.Store.List()
	s.check(err)
	return list
}

func (s checkedMemoryStore) ListAll() []Memory {
	s.t.Helper()
	list, err := s.Store.ListAll()
	s.check(err)
	return list
}

func (s checkedMemoryStore) ListArchived() []ArchivedMemory {
	s.t.Helper()
	list, err := s.Store.ListArchived()
	s.check(err)
	return list
}

func (s checkedMemoryStore) Revisions(ref string) []Memory {
	s.t.Helper()
	list, err := s.Store.Revisions(ref)
	s.check(err)
	return list
}

func (s checkedMemoryStore) Index() string {
	s.t.Helper()
	index, err := s.Store.Index()
	s.check(err)
	return index
}

func (s checkedMemoryStore) indexAt(now time.Time) string {
	s.t.Helper()
	index, err := s.Store.indexAt(now)
	s.check(err)
	return index
}

func (s checkedMemoryStore) pinnedGuidanceForProject() []Memory {
	s.t.Helper()
	pinned, err := s.Store.pinnedGuidanceForProject()
	s.check(err)
	return pinned
}

func (s checkedMemoryStore) recallIndex() *RecallIndex {
	s.t.Helper()
	index, err := BuildRecallIndex(s.Store)
	s.check(err)
	return index
}
