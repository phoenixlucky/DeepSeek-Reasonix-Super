// Package gitpush sends the current branch to its upstream, and nothing else.
//
// A push is the one git action Reasonix performs that leaves the machine, so it
// is narrower than the rest. It runs from a throwaway repository that holds no
// configuration of its own and borrows the real repository's objects through
// alternates, which means nothing the repository's .git/config says reaches it:
// no core.sshCommand, credential helper, url rewrite or hook of the repository
// can run or redirect it. Configuration at the user and system level, the
// person's own credential helper and ssh settings among it, applies as usual.
// Only https, ssh (including the scp spelling) and git:// remotes are accepted,
// enforced by git's own GIT_ALLOW_PROTOCOL as well as checked before anything
// runs. The push is never forced and names one branch, the upstream the branch
// already has. The remote shown to the person is the one after the user's own
// rewrites, so what they confirm is where it goes.
package gitpush
