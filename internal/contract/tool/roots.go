// roots.go — the directories a call acts under, and the typed cause when one is gone.
package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
)

// RootKind says which kind of directory a call needs.
type RootKind string

const (
	// RootWorkspace is the session's project folder.
	RootWorkspace RootKind = "workspace"
	// RootExternal is a folder the person granted from outside the project.
	RootExternal RootKind = "external"
)

// CodeRootMissing identifies a call that was not run because a directory it
// needs no longer exists. It is the host's own finding, not a permission or
// argument problem.
const CodeRootMissing = "tool.root_missing"

// Root is one directory a call acts under. Path is where the host looks; Display
// is what the model and the UI are told, so an external folder is named by the
// token it was granted under and never by the real path.
type Root struct {
	Kind    RootKind
	Path    string
	Display string
}

// RootUser is implemented by a tool that says which directories one call acts
// under. The host asks it before the call; it never reads the answer off a name.
type RootUser interface {
	RootsFor(args json.RawMessage) []Root
}

// RootsOf returns the directories t says a call with args acts under.
func RootsOf(t Tool, args json.RawMessage) []Root {
	if u, ok := t.(RootUser); ok {
		return u.RootsFor(args)
	}
	return nil
}

// ErrRootMissing matches every RootMissingError.
var ErrRootMissing = errors.New("a directory the call needs is missing")

// RootMissingError is the typed cause carried to the model and every frontend
// when a directory a call needs does not exist.
type RootMissingError struct{ Root Root }

func (e *RootMissingError) Error() string {
	what := "project folder"
	if e.Root.Kind == RootExternal {
		what = "external folder"
	}
	return fmt.Sprintf("root missing: the %s %s does not exist. This is not a permission or argument problem: it was moved, renamed or deleted, or its drive is not available. Nothing was run.", what, e.Root.Display)
}

func (e *RootMissingError) Is(target error) bool { return target == ErrRootMissing }

// RefusalCode lets a frontend classify the failure without reading its sentence.
func (e *RootMissingError) RefusalCode() string { return CodeRootMissing }

// Which names the kind of directory that is gone.
func (e *RootMissingError) Which() RootKind { return e.Root.Kind }

// MissingRoot returns the first of roots that does not exist as a directory, as
// a *RootMissingError. A root that cannot be read for another reason (a denied
// stat) is not missing: the tool reports that itself.
func MissingRoot(roots []Root, stat func(string) (fs.FileInfo, error)) error {
	for _, r := range roots {
		if r.Path == "" {
			continue
		}
		info, err := stat(r.Path)
		switch {
		case err == nil && !info.IsDir():
			return &RootMissingError{Root: r}
		case errors.Is(err, fs.ErrNotExist):
			return &RootMissingError{Root: r}
		}
	}
	return nil
}
