package skill

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestWriterIsVisibleToTheNextListingWhileAScanIsRunning(t *testing.T) {
	s, _, _ := cacheStore(t)
	s.scans.now = nil // real clock
	for i := range 120 {
		s.scans.drop()
		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					names(s)
				}
			}
		})
		name := fmt.Sprintf("s%d", i)
		time.Sleep(time.Millisecond)
		if _, err := s.CreateWithContent(name, ScopeGlobal, "---\nname: "+name+"\ndescription: d\n---\nbody\n"); err != nil {
			t.Fatal(err)
		}
		visible := names(s)[name]
		close(stop)
		wg.Wait()
		if !visible {
			t.Fatalf("round %d: %s missing right after CreateWithContent returned", i, name)
		}
	}
}
