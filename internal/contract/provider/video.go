package provider

import (
	"fmt"
	"slices"
	"strings"
)

// VideoRef names a video file the host read for the model. It carries no bytes:
// a transcript must not hold a hundred megabytes per read, and the file on disk
// is the one copy.
type VideoRef struct {
	// URL, when set, is a public address the vendor itself visits. Such a
	// reference has no file: Path, SHA256 and Size are empty.
	URL        string `json:"url,omitempty"`
	Path       string `json:"path"`
	Name       string `json:"name"`
	SHA256     string `json:"sha256"`
	MIME       string `json:"mime"`
	Size       int64  `json:"size"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// VideoShownNote is what every request after the model's answer says in place of
// a video it was shown once, so the model neither guesses why it is gone nor
// assumes it can still see it.
const VideoShownNote = "\n[video %s was shown to you when it was read and is not attached again; call read_video to look at it again]"

// VideoNotSentNote is what a message says when its provider cannot take video.
const VideoNotSentNote = "\n[video %s was not sent: this model cannot take video natively]"

// WithoutVideos returns m with its videos replaced by note, one line per video,
// for a provider that has no wire for them. note takes the file name.
func WithoutVideos(m Message, note string) Message {
	if len(m.Videos) == 0 {
		return m
	}
	for _, v := range m.Videos {
		m.Content += fmt.Sprintf(note, v.Name)
	}
	m.Videos = nil
	return m
}

// answeredAfter marks, for each message, whether an assistant message follows it.
func answeredAfter(msgs []Message) []bool {
	out := make([]bool, len(msgs))
	seen := false
	for i, m := range slices.Backward(msgs) {
		out[i] = seen
		if m.Role == RoleAssistant {
			seen = true
		}
	}
	return out
}

// How a video file reaches a model that reads video natively. A protocol is a
// property of an endpoint's wire, so it is declared beside the models and never
// read off a host name or a model name.
const (
	// VideoProtocolInlineDataURL sends the file in the request itself, as a
	// video_url content part holding a data URL.
	VideoProtocolInlineDataURL = "inline_data_url"
	// VideoProtocolMoonshotFile uploads the file once and refers to it by id in
	// a video_url content part.
	VideoProtocolMoonshotFile = "moonshot_file"
	// VideoProtocolGeminiInline sends the file in the request as a video content
	// block holding base64 data and a MIME type.
	VideoProtocolGeminiInline = "gemini_inline"
	// VideoProtocolGeminiFiles uploads the file to Google's Files API, refers to
	// it by its uri in a video content block, and deletes it once the request ends.
	VideoProtocolGeminiFiles = "gemini_files"
	// VideoProtocolZhipuURL names a public HTTPS address in a video_url content
	// part and nothing else: Zhipu's page shows no base64 or file form for video
	// (docs.bigmodel.cn, GLM-4.6V, read 2026-10-17).
	VideoProtocolZhipuURL = "zhipu_url"
)

// The ceilings the vendors document for each protocol, read from their own
// pages on 2026-10-17.
const (
	// VideoInlineEncodedLimit is DashScope's bound on a base64 video string
	// ("编码后的字符串小于 10MB"), taken as decimal megabytes.
	VideoInlineEncodedLimit = 10_000_000
	// VideoUploadLimit is Moonshot's bound on one uploaded file (100 MiB).
	VideoUploadLimit = 100 << 20
	// VideoGeminiInlineRequestLimit is Google's bound for inline video: "suitable
	// for shorter videos under 20MB total request size" (video-understanding page).
	// Its table says 100 MB for the same input, and the two are not reconciled, so
	// the narrower reading stands. The bound is on the whole request, which carries
	// the file base64-encoded, so the raw file is held to three quarters of it.
	VideoGeminiInlineRequestLimit = 20_000_000
	// VideoGeminiFileLimit is the Files API's per-file maximum, 2 GB.
	VideoGeminiFileLimit = 2 << 30
)

// VideoProtocolLimit is the documented ceiling on the raw file for a protocol,
// with the name that says where it comes from.
func VideoProtocolLimit(protocol string) (source string, bytes int64) {
	switch protocol {
	case VideoProtocolInlineDataURL:
		return "inline_data_url", VideoInlineEncodedLimit / 4 * 3
	case VideoProtocolMoonshotFile:
		return "moonshot_file", VideoUploadLimit
	case VideoProtocolGeminiInline:
		return "gemini_inline", VideoGeminiInlineRequestLimit / 4 * 3
	case VideoProtocolGeminiFiles:
		return "gemini_files", VideoGeminiFileLimit
	case VideoProtocolZhipuURL:
		return "zhipu_url", 0
	}
	return "", 0
}

// VideoPart is a video resolved for one request: where the vendor reads it from,
// a data URL or a reference to a file it holds. It exists only while a request
// is built and is never stored.
type VideoPart struct {
	Name string
	URL  string
	// MIME is the container type, for a wire that names it beside the data.
	MIME string
}

// VideoFailedNote is what a message says when its video could not be sent; code
// is the host's identity for why.
const VideoFailedNote = "\n[video %s was not sent (%s)]"

// SafeVideoName keeps a file name on one line of a note.
func SafeVideoName(name string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "]", ")").Replace(name)
}

// WithoutVideosAll applies WithoutVideos to every message, leaving the slice as
// it was when none carries a video.
func WithoutVideosAll(msgs []Message, note string) []Message {
	var out []Message
	for i, m := range msgs {
		if len(m.Videos) == 0 {
			continue
		}
		if out == nil {
			out = append([]Message(nil), msgs...)
		}
		out[i] = WithoutVideos(m, note)
	}
	if out == nil {
		return msgs
	}
	return out
}
