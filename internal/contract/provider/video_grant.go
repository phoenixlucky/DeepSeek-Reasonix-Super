package provider

import "sync"

var videoGrants = struct {
	mu    sync.Mutex
	paths map[string]string
}{paths: map[string]string{}}

// GrantVideo records, in this process only, the path a tool checked and read a
// video from, under the file's hash. Whatever opens a VideoRef later opens this
// path and never the one the reference carries, because a stored transcript can
// be edited, restored or imported with a path no tool ever saw.
func GrantVideo(ref VideoRef) {
	videoGrants.mu.Lock()
	defer videoGrants.mu.Unlock()
	videoGrants.paths[ref.SHA256] = ref.Path
}

// GrantedVideoPath is the path a tool in this process handed out for the hash.
func GrantedVideoPath(ref VideoRef) (string, bool) {
	videoGrants.mu.Lock()
	defer videoGrants.mu.Unlock()
	path, ok := videoGrants.paths[ref.SHA256]
	return path, ok
}
