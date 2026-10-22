// Package persona adds user-authored prefix and suffix text around first-party
// guidance without replacing it. Optional runtime context is independently
// controlled. The cache-stable result is assembled at boot; saved changes apply
// to new sessions and the next rebuild of an existing session.
package persona
