package skill

import "testing"

func TestFrozenSkillsUseOneSnapshotAcrossFreshAndCachedDiscovery(t *testing.T) {
	s, _, _ := cacheStore(t)
	if _, err := s.CreateWithContent("alpha", ScopeGlobal, "---\nname: alpha\ndescription: a\n---\noriginal\n"); err != nil {
		t.Fatal(err)
	}
	if !names(s)["alpha"] {
		t.Fatal("live cached listing missing alpha")
	}
	frozen := s.Freeze(s.CompositionSkills())
	if _, err := s.CreateWithContent("beta", ScopeGlobal, "---\nname: beta\ndescription: b\n---\nnew\n"); err != nil {
		t.Fatal(err)
	}
	if !names(s)["beta"] {
		t.Fatal("live listing did not invalidate")
	}
	for _, list := range [][]Skill{frozen.List(), frozen.SlashList(), frozen.CompositionSkills()} {
		if len(list) != 1 || list[0].Name != "alpha" {
			t.Fatalf("discovery bypassed frozen composition: %+v", list)
		}
	}
}
