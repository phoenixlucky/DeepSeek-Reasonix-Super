package gemini

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"reasonix/internal/contract/provider"
)

// Inline and Files API video follow https://ai.google.dev/gemini-api/docs/video-understanding
// and /files (read 2026-10-17); not run against the real endpoint.

// afterVideoOpen and beforeVideoOpen let a test move the path around the open.
var (
	afterVideoOpen  = func(string) {}
	beforeVideoOpen = func(string) {}
)

// fileWait bounds the poll for a file to become ACTIVE.
var (
	filePollEvery = 2 * time.Second
	filePollFor   = 3 * time.Minute
)

var errVideoChanged = errors.New("the file no longer matches what the tool read")

// errUntrustedResponse is an upload answer that points somewhere this adapter
// did not configure, or names a file in a shape Google does not issue.
var errUntrustedResponse = errors.New("the upload answer is not one this adapter follows")

// fileName is the only shape a file resource name takes here; it is joined onto
// the API root for polling and deletion, so anything else would let a response
// choose which path is requested.
var fileName = regexp.MustCompile(`^files/[A-Za-z0-9-]+$`)

// videoWire sends the videos one request carries on the protocol the entry
// declares.
type videoWire struct {
	protocol  string
	root      string // {origin}/{version}, where file resources live
	uploadURL string // {origin}/upload/{version}/files
}

func newVideoWire(protocol, root string) *videoWire {
	switch protocol {
	case provider.VideoProtocolGeminiInline, provider.VideoProtocolGeminiFiles:
	default:
		return nil
	}
	u, err := url.Parse(root)
	if err != nil || u.Host == "" {
		return nil
	}
	upload := *u
	upload.Path = "/upload" + strings.TrimRight(u.Path, "/") + "/files"
	return &videoWire{protocol: protocol, root: strings.TrimRight(root, "/"), uploadURL: upload.String()}
}

// resolveVideos replaces every video the request carries with what Google can
// read, or with a note naming why not. uploaded names the files this request put
// on Google's side, which the caller deletes when the request is over.
func (c *client) resolveVideos(ctx context.Context, msgs []provider.Message) (out []provider.Message, uploaded []string) {
	out = msgs
	for i, m := range msgs {
		if len(m.Videos) == 0 {
			continue
		}
		if &out[0] == &msgs[0] {
			out = append([]provider.Message(nil), msgs...)
		}
		if c.video == nil {
			out[i] = provider.WithoutVideos(m, provider.VideoNotSentNote)
			continue
		}
		m.VideoParts = append([]provider.VideoPart(nil), m.VideoParts...)
		for _, ref := range m.Videos {
			part, name, code := c.video.resolve(ctx, c, ref)
			if name != "" {
				uploaded = append(uploaded, name)
			}
			if code != "" {
				m.Content += fmt.Sprintf(provider.VideoFailedNote, provider.SafeVideoName(ref.Name), code)
				continue
			}
			m.VideoParts = append(m.VideoParts, part)
		}
		m.Videos = nil
		out[i] = m
	}
	return out, uploaded
}

// resolve opens the file a tool in this process granted, once, at the path the
// tool checked, and checks it end to end before a byte goes out. A reference
// never granted is reported as unavailable.
func (w *videoWire) resolve(ctx context.Context, c *client, ref provider.VideoRef) (part provider.VideoPart, uploaded string, code string) {
	if ref.URL != "" {
		// A public YouTube address Google reads itself; only one a tool granted.
		switch {
		case !provider.VideoAddressGranted(ref.URL):
			return part, "", provider.VideoURLUnavailable
		case !provider.VideoAddressAccepted(w.protocol, ref.URL):
			return part, "", provider.VideoURLUnsupported
		}
		return provider.VideoPart{Name: ref.Name, URL: ref.URL}, "", ""
	}
	_, limit := provider.VideoProtocolLimit(w.protocol)
	if ref.Size > limit {
		return part, "", provider.VideoTooLarge
	}
	path, granted := provider.GrantedVideoPath(ref)
	if !granted {
		return part, "", provider.VideoUnavailable
	}
	beforeVideoOpen(path)
	f, err := os.Open(path)
	if err != nil {
		return part, "", readCode(err)
	}
	defer f.Close()
	afterVideoOpen(path)
	if w.protocol == provider.VideoProtocolGeminiInline {
		raw, err := io.ReadAll(io.LimitReader(f, ref.Size+1))
		if err != nil {
			return part, "", provider.VideoReadFailed
		}
		if sum := sha256.Sum256(raw); int64(len(raw)) != ref.Size || hex.EncodeToString(sum[:]) != ref.SHA256 {
			return part, "", provider.VideoFileChanged
		}
		return provider.VideoPart{Name: ref.Name, URL: base64.StdEncoding.EncodeToString(raw), MIME: ref.MIME}, "", ""
	}
	if code := verifyOpened(f, ref); code != "" {
		return part, "", code
	}
	file, err := w.upload(ctx, c, ref, f)
	if file.Name != "" {
		uploaded = file.Name
	}
	switch {
	case errors.Is(err, errVideoChanged):
		return part, uploaded, provider.VideoFileChanged
	case err != nil:
		return part, uploaded, provider.VideoUploadFailed
	}
	return provider.VideoPart{Name: ref.Name, URL: file.URI, MIME: ref.MIME}, uploaded, ""
}

func verifyOpened(f *os.File, ref provider.VideoRef) string {
	sum := sha256.New()
	n, err := io.Copy(sum, f)
	if err != nil {
		return provider.VideoReadFailed
	}
	if n != ref.Size || hex.EncodeToString(sum.Sum(nil)) != ref.SHA256 {
		return provider.VideoFileChanged
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return provider.VideoReadFailed
	}
	return ""
}

func readCode(err error) string {
	if errors.Is(err, fs.ErrNotExist) {
		return provider.VideoFileMissing
	}
	return provider.VideoReadFailed
}

// uploadedFile is the part of a Files API resource this adapter reads.
type uploadedFile struct {
	Name  string `json:"name"`
	URI   string `json:"uri"`
	State string `json:"state"`
}

// fileEnvelope reads either shape a Files API answer may take: the resource
// itself, or the resource under "file".
type fileEnvelope struct {
	uploadedFile
	File *uploadedFile `json:"file"`
}

func (e fileEnvelope) resource() uploadedFile {
	if e.File != nil {
		return *e.File
	}
	return e.uploadedFile
}

// sameOrigin says an address Google handed back is on the host this adapter was
// configured to upload to, with the same scheme, so the file's bytes and the key
// never leave for a host a response chose.
func (w *videoWire) sameOrigin(raw string) bool {
	got, err := url.Parse(raw)
	want, werr := url.Parse(w.uploadURL)
	return err == nil && werr == nil && got.Scheme == want.Scheme && got.Host == want.Host && got.User == nil
}

func (w *videoWire) authed(c *client, req *http.Request) {
	req.Header.Set("x-goog-api-key", c.apiKey())
	for name, value := range c.headers {
		req.Header.Set(name, value)
	}
	provider.ApplyClientIdentity(req)
}

// upload runs the resumable protocol and waits for the file to be ACTIVE. The
// bytes are hashed as they go; a file that changed under the upload is deleted
// by the caller through the name this returns.
func (w *videoWire) upload(ctx context.Context, c *client, ref provider.VideoRef, f *os.File) (uploadedFile, error) {
	startBody, _ := json.Marshal(map[string]any{"file": map[string]string{"display_name": ref.Name}})
	start, err := http.NewRequestWithContext(ctx, http.MethodPost, w.uploadURL, strings.NewReader(string(startBody)))
	if err != nil {
		return uploadedFile{}, err
	}
	w.authed(c, start)
	start.Header.Set("Content-Type", "application/json")
	start.Header.Set("X-Goog-Upload-Protocol", "resumable")
	start.Header.Set("X-Goog-Upload-Command", "start")
	start.Header.Set("X-Goog-Upload-Header-Content-Length", strconv.FormatInt(ref.Size, 10))
	start.Header.Set("X-Goog-Upload-Header-Content-Type", ref.MIME)
	resp, err := c.http.Do(start)
	if err != nil {
		return uploadedFile{}, err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
	target := resp.Header.Get("X-Goog-Upload-URL")
	if resp.StatusCode != http.StatusOK || target == "" {
		return uploadedFile{}, fmt.Errorf("upload start: HTTP %d", resp.StatusCode)
	}
	if !w.sameOrigin(target) {
		return uploadedFile{}, errUntrustedResponse
	}

	sum := sha256.New()
	body := io.TeeReader(f, sum)
	put, err := http.NewRequestWithContext(ctx, http.MethodPost, target, body)
	if err != nil {
		return uploadedFile{}, err
	}
	put.ContentLength = ref.Size
	put.Header.Set("X-Goog-Upload-Offset", "0")
	put.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	resp, err = c.http.Do(put)
	if err != nil {
		return uploadedFile{}, err
	}
	var env fileEnvelope
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env)
	resp.Body.Close()
	file := env.resource()
	if hex.EncodeToString(sum.Sum(nil)) != ref.SHA256 {
		return file, errVideoChanged
	}
	if resp.StatusCode != http.StatusOK || err != nil || file.Name == "" {
		return file, fmt.Errorf("upload finalize: HTTP %d", resp.StatusCode)
	}
	if !fileName.MatchString(file.Name) {
		return uploadedFile{}, errUntrustedResponse
	}
	return w.waitActive(ctx, c, file)
}

// waitActive polls the file until Google says it can be read. A state this
// adapter does not know is a failure, not a reason to keep waiting.
func (w *videoWire) waitActive(ctx context.Context, c *client, file uploadedFile) (uploadedFile, error) {
	deadline := time.Now().Add(filePollFor)
	for {
		switch file.State {
		case "ACTIVE":
			if file.URI == "" {
				return file, errors.New("the file is active and has no uri")
			}
			return file, nil
		case "PROCESSING":
		default:
			return file, fmt.Errorf("the file is in state %q", file.State)
		}
		if time.Now().After(deadline) {
			return file, errors.New("the file did not become active in time")
		}
		select {
		case <-ctx.Done():
			return file, ctx.Err()
		case <-time.After(filePollEvery):
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.root+"/"+file.Name, nil)
		if err != nil {
			return file, err
		}
		w.authed(c, req)
		resp, err := c.http.Do(req)
		if err != nil {
			return file, err
		}
		var env fileEnvelope
		err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			return file, fmt.Errorf("file status: HTTP %d", resp.StatusCode)
		}
		next := env.resource()
		if next.Name != "" && next.Name != file.Name {
			return file, errUntrustedResponse
		}
		next.Name = file.Name
		file = next
	}
}

// deleteFiles removes what a request uploaded, best effort: Google's own 48-hour
// expiry is the backstop, not the plan.
func (c *client) deleteFiles(names []string) {
	if c.video == nil || len(names) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, name := range names {
		if !fileName.MatchString(name) {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.video.root+"/"+name, nil)
		if err != nil {
			continue
		}
		c.video.authed(c, req)
		if resp, err := c.http.Do(req); err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
		}
	}
}
