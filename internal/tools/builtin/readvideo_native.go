package builtin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/settings"
	"reasonix/internal/contract/tool"
)

// Identities of the refusals a native read owns, beside the ones read_video
// already has.
const (
	CodeVideoNativeUnsupported = "video.native_unsupported_model"
	CodeVideoNativeNoProtocol  = "video.native_no_protocol"
	CodeVideoNativeTooLarge    = "video.native_too_large"
	CodeVideoNativeContainer   = "video.native_container_unsupported"
	CodeVideoNativeAddressOnly = provider.VideoAddressOnly
	CodeVideoURLDisabled       = "video.url_disabled"
	CodeVideoURLInvalid        = "video.url_invalid"
	CodeVideoURLUnsupported    = "video.url_unsupported_model"
	CodeVideoURLNotFromUser    = "video.url_not_from_user"
)

// nativeContainer names the container a file is and says whether the protocol
// sends it natively. DashScope's and Moonshot's pages show MP4 and say nothing a
// container beside it can be trusted on. Google lists MP4, WebM and MOV (the
// MIME types it writes are video/mp4, video/webm and video/mov), so those three
// are sent on its protocols and every other container is sampled into frames.
func nativeContainer(head []byte, protocol string) (mime string, accepted string, ok bool) {
	kind := ""
	switch {
	case isMP4(head):
		kind = "video/mp4"
	case isQuickTime(head):
		kind = "video/mov"
	case isWebM(head):
		kind = "video/webm"
	}
	switch protocol {
	case provider.VideoProtocolGeminiInline, provider.VideoProtocolGeminiFiles:
		return kind, "MP4, WebM or MOV", kind != ""
	}
	return kind, "MP4", kind == "video/mp4"
}

// ExecuteWithVideo reads a video the way the mode asks. frames samples stills with
// ffmpeg; native hands the file to a model that watches video, with no decoder
// in between; auto does native when the selected model offers it and the file
// fits, and otherwise samples frames and says why.
func (r readVideo) ExecuteWithVideo(ctx context.Context, args json.RawMessage) (string, []string, []provider.VideoRef, error) {
	var p struct {
		Path string `json:"path"`
		URL  string `json:"url"`
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", nil, nil, fmt.Errorf("invalid args: %w", err)
	}
	if p.URL != "" {
		if p.Path != "" {
			return "", nil, nil, errors.New("give a path or a url, not both")
		}
		text, ref, err := address(ctx, p.URL)
		if err != nil {
			return "", nil, nil, err
		}
		return text, nil, []provider.VideoRef{ref}, nil
	}
	if p.Path == "" {
		return "", nil, nil, errors.New("path is required")
	}
	mode := strings.ToLower(strings.TrimSpace(p.Mode))
	switch mode {
	case "", "auto", "frames", "native":
	default:
		return "", nil, nil, fmt.Errorf("mode must be auto, frames or native, not %q", p.Mode)
	}
	frames := func(note string) (string, []string, []provider.VideoRef, error) {
		text, images, err := r.ExecuteWithImages(ctx, args)
		if err == nil && note != "" {
			text = note + "\n" + text
		}
		return text, images, nil, err
	}
	if mode == "frames" {
		return frames("")
	}
	plan, ok := tool.NativeVideoFrom(ctx)
	if !ok {
		if mode == "native" {
			code := plan.Unavailable
			if code == "" {
				code = CodeVideoNativeUnsupported
			}
			return "", nil, nil, tool.Refusal{Code: code, Message: "the selected model does not take video natively: declare it in video_models and name its video_protocol in the provider's settings, or use mode frames"}
		}
		return frames("")
	}
	text, ref, err := r.native(p.Path, plan)
	if err == nil {
		return text, nil, []provider.VideoRef{ref}, nil
	}
	var refusal tool.Refusal
	if mode != "native" && errors.As(err, &refusal) && (refusal.Code == CodeVideoNativeTooLarge || refusal.Code == CodeVideoNativeContainer || refusal.Code == CodeVideoNativeAddressOnly) {
		return frames(fmt.Sprintf("[native video not used (%s): %s; frames were sampled instead]", refusal.Code, refusal.Message))
	}
	return "", nil, nil, err
}

func (r readVideo) native(path string, plan tool.NativeVideo) (string, provider.VideoRef, error) {
	if !provider.VideoProtocolTakesFiles(plan.Protocol) {
		return "", provider.VideoRef{}, tool.Refusal{Code: CodeVideoNativeAddressOnly, Message: "this model's video protocol takes a public address and no file: give a url, or use mode frames"}
	}
	rp := resolveReadablePath(r.workDir, path, r.paths)
	if err := r.refuse(rp); err != nil {
		return "", provider.VideoRef{}, err
	}
	f, err := os.Open(rp.Path)
	if err != nil {
		return "", provider.VideoRef{}, fmt.Errorf("read %s: %w", rp.DisplayPath, err)
	}
	defer f.Close()
	if err := confineOpened(r.readRoots, f); err != nil {
		return "", provider.VideoRef{}, err
	}
	st, err := f.Stat()
	if err != nil {
		return "", provider.VideoRef{}, fmt.Errorf("read %s: %w", rp.DisplayPath, err)
	}
	if st.IsDir() {
		return "", provider.VideoRef{}, fmt.Errorf("%s is a directory, not a video file", rp.DisplayPath)
	}
	head := make([]byte, readVideoSniffBytes)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	mime, accepted, sendable := nativeContainer(head, plan.Protocol)
	switch {
	case videoDemuxer(head) == "":
		return "", provider.VideoRef{}, tool.Refusal{Code: CodeVideoUnsupported, Message: fmt.Sprintf("%s is not a video container read_video recognises", rp.DisplayPath)}
	case !sendable:
		return "", provider.VideoRef{}, tool.Refusal{Code: CodeVideoNativeContainer, Message: fmt.Sprintf("%s is not %s, the only containers sent natively on this protocol", rp.DisplayPath, accepted)}
	}
	if limit := plan.Tightest(); limit.Bytes > 0 && st.Size() > limit.Bytes {
		return "", provider.VideoRef{}, tool.Refusal{Code: CodeVideoNativeTooLarge, Message: fmt.Sprintf("%s is %d bytes; the %s limit for sending video natively is %d bytes", rp.DisplayPath, st.Size(), limit.Source, limit.Bytes)}
	}
	sum := sha256.New()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", provider.VideoRef{}, err
	}
	if _, err := io.Copy(sum, f); err != nil {
		return "", provider.VideoRef{}, fmt.Errorf("read %s: %w", rp.DisplayPath, err)
	}
	abs, err := filepath.Abs(rp.Path)
	if err != nil {
		abs = rp.Path
	}
	ref := provider.VideoRef{
		Path: abs, Name: filepath.Base(rp.Path), SHA256: hex.EncodeToString(sum.Sum(nil)),
		MIME: mime, Size: st.Size(), DurationMs: mp4DurationMs(f, st.Size()),
	}
	provider.GrantVideo(ref)
	var b strings.Builder
	fmt.Fprintf(&b, "video %s: %d bytes", rp.DisplayPath, ref.Size)
	if ref.DurationMs > 0 {
		fmt.Fprintf(&b, ", %.1fs", float64(ref.DurationMs)/1000)
	}
	fmt.Fprintf(&b, ". Handed to %s as a video file, not sampled: it is attached to the next request only; call read_video again to look at it again.", plan.Model)
	return b.String(), ref, nil
}

// isMP4 reports an ISO base media file that is not QuickTime.
func isMP4(head []byte) bool {
	return len(head) >= 12 && string(head[4:8]) == "ftyp" && string(head[8:12]) != "qt  "
}

// isQuickTime reports an ISO base media file whose major brand is QuickTime.
func isQuickTime(head []byte) bool {
	return len(head) >= 12 && string(head[4:8]) == "ftyp" && string(head[8:12]) == "qt  "
}

// isWebM reports a Matroska file whose EBML header names the webm doctype. An
// .mkv shares the magic bytes and not the doctype.
func isWebM(head []byte) bool {
	return len(head) >= 4 && head[0] == 0x1a && head[1] == 0x45 && head[2] == 0xdf && head[3] == 0xa3 &&
		bytes.Contains(head[:min(len(head), 64)], []byte{0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'})
}

// mp4DurationMs reads the movie header's duration from the box tree without
// decoding anything, or 0 when the tree does not say.
func mp4DurationMs(f *os.File, size int64) int64 {
	moov, ok := findBox(f, 0, size, "moov")
	if !ok {
		return 0
	}
	mvhd, ok := findBox(f, moov.start, moov.end, "mvhd")
	if !ok {
		return 0
	}
	buf := make([]byte, 32)
	if _, err := f.ReadAt(buf, mvhd.start); err != nil {
		return 0
	}
	var scale, dur uint64
	if buf[0] == 1 {
		scale = uint64(binary.BigEndian.Uint32(buf[20:]))
		dur = binary.BigEndian.Uint64(buf[24:])
	} else {
		scale = uint64(binary.BigEndian.Uint32(buf[12:]))
		dur = uint64(binary.BigEndian.Uint32(buf[16:]))
	}
	if scale == 0 || dur == 0 {
		return 0
	}
	return int64(dur * 1000 / scale)
}

type boxSpan struct{ start, end int64 }

// findBox walks the sibling boxes in [from, to) and returns the payload span of
// the first named kind. Only headers are read, so a damaged file costs a few
// bytes of reading and answers "not found".
func findBox(f *os.File, from, to int64, kind string) (boxSpan, bool) {
	hdr := make([]byte, 16)
	for at := from; at+8 <= to; {
		if _, err := f.ReadAt(hdr[:8], at); err != nil {
			return boxSpan{}, false
		}
		size, header := int64(binary.BigEndian.Uint32(hdr)), int64(8)
		switch size {
		case 1:
			if _, err := f.ReadAt(hdr[8:16], at+8); err != nil {
				return boxSpan{}, false
			}
			size, header = int64(binary.BigEndian.Uint64(hdr[8:])), 16
		case 0:
			size = to - at
		}
		if size < header || at+size > to {
			return boxSpan{}, false
		}
		if string(hdr[4:8]) == kind {
			return boxSpan{start: at + header, end: at + size}, true
		}
		at += size
	}
	return boxSpan{}, false
}

// address hands a public video address to the vendor, which visits it itself:
// nothing is downloaded here, so there is no frame fallback. It is sent only when
// the user turned the switch on, the model's protocol documents addresses, and
// the address is one the user wrote in this conversation, so text the model read
// elsewhere cannot make a vendor visit an address its author chose.
func address(ctx context.Context, raw string) (string, provider.VideoRef, error) {
	plan, ok := tool.NativeVideoFrom(ctx)
	if !ok {
		code := plan.Unavailable
		if code == "" {
			code = CodeVideoNativeUnsupported
		}
		return "", provider.VideoRef{}, tool.Refusal{Code: code, Message: "the selected model does not take video natively, and an address cannot be sampled into frames"}
	}
	switch {
	case !plan.Addresses:
		return "", provider.VideoRef{}, tool.Refusal{Code: CodeVideoURLDisabled, Remedy: settings.RemedyFor("tools.video_url_passthrough", true),
			Message: "handing an address to the model vendor is off: the user has to turn it on, because the vendor then visits the address itself"}
	case provider.ValidVideoAddress(raw) != nil:
		return "", provider.VideoRef{}, tool.Refusal{Code: CodeVideoURLInvalid, Message: "the address must be a public https address with a host name, no credentials, and at most 2048 characters"}
	case !provider.VideoAddressAccepted(plan.Protocol, raw):
		return "", provider.VideoRef{}, tool.Refusal{Code: CodeVideoURLUnsupported, Message: "the selected model's video protocol does not take this address (Zhipu takes a public https address; Gemini only a YouTube address)"}
	case !slices.Contains(tool.UserAddressesFrom(ctx), raw):
		return "", provider.VideoRef{}, tool.Refusal{Code: CodeVideoURLNotFromUser, Message: "only an address the user wrote in this conversation can be handed to the vendor; ask the user for it"}
	}
	provider.GrantVideoAddress(raw)
	name := raw
	if u, err := url.Parse(raw); err == nil {
		name = u.Host + u.EscapedPath()
	}
	ref := provider.VideoRef{URL: raw, Name: name}
	return fmt.Sprintf("video address %s handed to %s, which visits it itself; nothing was downloaded here. It is attached to the next request only; call read_video again to look at it again.", name, plan.Model), ref, nil
}
