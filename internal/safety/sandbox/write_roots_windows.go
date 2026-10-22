package sandbox

// backendWriteDirs are the toolchain caches the Windows token may also write,
// beside the caller's roots. The session temp is granted at launch instead.
func backendWriteDirs(spec Spec) []string {
	if spec.MinimalWrites {
		return nil
	}
	return windowsCacheDirs(spec)
}
