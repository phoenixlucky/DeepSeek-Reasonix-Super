package sandbox

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sessionTempPrefix names the per-lease directories the host makes under the
// temp root (internal/state/sessiontemp); a test holds the two spellings equal.
const sessionTempPrefix = "reasonix-session-tmp-"

// errLaunchRefused marks a launcher request the host did not make.
var errLaunchRefused = errors.New("launch request refused")

// checkWinLaunch holds a request to what the host intends: every Grant entry is
// a session temp directory directly under the temp root. Who may ask at all is
// decided by the token the launcher runs under (see launcherMayRun), not by
// anything the request says: argv, environment and files are all the caller's own.
func checkWinLaunch(l winLaunch) error {
	base, _ := resolveWithLinks(os.TempDir())
	for _, dir := range l.Grant {
		real, _ := resolveWithLinks(dir)
		// The launcher may already run with TEMP pointing at the lease itself.
		inBase := strings.EqualFold(filepath.Dir(real), base) || strings.EqualFold(real, base)
		if !inBase || !strings.HasPrefix(filepath.Base(real), sessionTempPrefix) {
			return fmt.Errorf("%w: %s is not a session temp directory", errLaunchRefused, dir)
		}
	}
	return nil
}

// winLaunchFlag marks an argv as a request to run the rest of it under the
// write-restricted token. It is an argument of whichever host binary imports
// this package, so no helper has to be shipped and found.
const winLaunchFlag = "--reasonix-win-sandbox"

// winLaunch is what the launcher needs beyond the command: the SID to restrict
// the token to and the environment the host overrides. It carries no secret.
type winLaunch struct {
	SID string            `json:"sid"`
	Env map[string]string `json:"env,omitempty"`
	// Grant lists directories the launcher grants before it starts the command.
	Grant []string `json:"grant,omitempty"`
}

// winLaunchArgv wraps argv so that running it starts self as the launcher.
func winLaunchArgv(self string, l winLaunch, argv []string) []string {
	raw, _ := json.Marshal(l)
	out := []string{self, winLaunchFlag, base64.RawURLEncoding.EncodeToString(raw), "--"}
	return append(out, argv...)
}

// parseWinLaunch reads a launcher request from os.Args-shaped input (program
// name first). ok is false for anything that is not one, so an ordinary start of
// the binary is untouched.
func parseWinLaunch(args []string) (l winLaunch, argv []string, ok bool) {
	if len(args) < 5 || args[1] != winLaunchFlag || args[3] != "--" {
		return winLaunch{}, nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(args[2])
	if err != nil || json.Unmarshal(raw, &l) != nil || !syntheticSID(l.SID) {
		return winLaunch{}, nil, false
	}
	return l, args[4:], true
}
