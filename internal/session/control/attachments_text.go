package control

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"reasonix/internal/base/textutil"
)

// MaxTextAttachmentBytes caps a pasted text kept as a file.
const MaxTextAttachmentBytes = 8 * 1024 * 1024

// ErrAttachmentNotText is a paste that is not UTF-8 text.
var ErrAttachmentNotText = fmt.Errorf("attachment is not UTF-8 text")

// SaveTextAttachmentInRoot stores a pasted text as a .txt under the attachment
// directory, by the same rules as every other attachment. The extension is
// fixed here, not taken from the client.
func SaveTextAttachmentInRoot(root, text string) (string, error) {
	if text == "" {
		return "", attachmentSizeError(0, MaxTextAttachmentBytes)
	}
	if len(text) > MaxTextAttachmentBytes {
		return "", attachmentSizeError(len(text), MaxTextAttachmentBytes)
	}
	if !utf8.ValidString(text) || strings.IndexByte(text, 0) >= 0 {
		return "", &AttachmentError{Kind: ErrAttachmentNotText, Msg: "pasted data is not UTF-8 text"}
	}
	return saveAttachmentBytesInRoot(root, ".txt", []byte(text))
}

// TextAttachmentPointer is what stands in the message for a kept paste: its
// size, where it is and how to read it. The text itself is not in it.
func TextAttachmentPointer(rel, text string) string {
	lines := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		lines++
	}
	return fmt.Sprintf("[pasted text kept out of this message: %d lines, %s]\nSaved: %s\nRead a window of it with read_file (offset/limit), or search it with grep. It is not in this message.\n",
		lines, textutil.HumanBytes(int64(len(text))), rel)
}
