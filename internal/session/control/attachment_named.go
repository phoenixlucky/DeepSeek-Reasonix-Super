// attachment_named.go — the host's own attachments a task text points at.
package control

import (
	"context"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"reasonix/internal/runtime/agent"
)

// attachmentsDir is where the host stores what a person pasted or dropped.
const attachmentsDir = ".reasonix/attachments"

// attachmentImagesNamedIn resolves the images a piece of task text names. The
// judgement is the path's: a token that resolves, inside this workspace, to an
// image file directly in the host's own attachments directory, read through the
// same checks an @-reference gets (no symlinks, bounded size, a real image).
// Nothing about the sentence around the path is read.
func (c *Controller) attachmentImagesNamedIn(text string) []string {
	if c.workspaceRoot == "" || !strings.Contains(filepath.ToSlash(text), attachmentsDir+"/") {
		return nil
	}
	var urls []string
	seen := map[string]bool{}
	for _, tok := range taskPathTokens(text) {
		rel, ok := workspaceRefPath(tok, c.workspaceRoot)
		if !ok || seen[rel] || path.Dir(rel) != attachmentsDir || !isImageAttachmentRef(rel) {
			continue
		}
		seen[rel] = true
		if url, err := visionImageDataURL(c.workspaceRoot, rel); err == nil {
			urls = append(urls, url)
		}
	}
	return urls
}

// taskPathTokens splits text into the pieces a path could be, stripping what
// prose puts around one: quotes, brackets, a leading @, a closing full stop.
func taskPathTokens(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', '\f', '"', '\'', '`', '<', '>', '(', ')', '[', ']', '{', '}', '|', ',', ';':
			return true
		}
		return false
	})
	var out []string
	for _, f := range fields {
		f = strings.TrimLeft(f, "@")
		f = strings.TrimRight(f, ".:!?")
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// bindTaskImages is what a delegation gets from agent.BindTaskImages: the
// images its task names join the ones the turn carried, and the vision routing
// is bound for them, so a child on a text-only model is moved to the vision
// role instead of dropping them.
func (c *Controller) bindTaskImages(ctx context.Context, task string) context.Context {
	named := c.attachmentImagesNamedIn(task)
	if len(named) == 0 {
		return ctx
	}
	have := agent.SubagentImageCandidates(ctx)
	merged := slices.Clone(have)
	for _, url := range named {
		if !slices.Contains(merged, url) {
			merged = append(merged, url)
		}
	}
	if len(merged) == len(have) {
		return ctx
	}
	return c.withVisionRouting(agent.WithSubagentImageCandidates(ctx, merged))
}
