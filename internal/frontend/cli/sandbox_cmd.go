package cli

import (
	"fmt"
	"os"

	"reasonix/internal/contract/config"
	"reasonix/internal/safety/sandbox"
)

// sandboxCommand is `reasonix sandbox list|revoke`: what the Windows sandbox has
// granted on this machine, and taking it back. It is not listed in usage; the
// guide documents it beside the setting that causes the grants.
func sandboxCommand(args []string) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "reasonix sandbox:", err)
		return 1
	}
	dir := cfg.Roots().SandboxDir()
	switch {
	case len(args) == 1 && args[0] == "list":
		sid, roots, err := sandbox.GrantedRoots(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "reasonix sandbox:", err)
			return 1
		}
		if len(roots) == 0 {
			fmt.Println("no folders are granted to the Windows sandbox")
			return 0
		}
		fmt.Println("Windows sandbox SID:", sid)
		for _, r := range roots {
			fmt.Println("  granted:", r)
		}
		return 0
	case len(args) == 1 && args[0] == "revoke":
		done, failed, err := sandbox.RevokeGrants(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "reasonix sandbox:", err)
			return 1
		}
		for _, r := range done {
			fmt.Println("revoked:", r)
		}
		for r, e := range failed {
			fmt.Fprintf(os.Stderr, "could not revoke %s: %v\n", r, e)
		}
		if len(failed) > 0 {
			return 1
		}
		if len(done) == 0 {
			fmt.Println("nothing to revoke")
		}
		return 0
	}
	fmt.Fprintln(os.Stderr, "usage: reasonix sandbox list | revoke   (Windows only: the folders the sandbox granted, and taking them back)")
	return 2
}
