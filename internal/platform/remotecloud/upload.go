package remotecloud

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// A request body larger than one relay message crosses as numbered parts and is
// handed to the desktop only when the last one has arrived. The 64 KiB read
// limit on the relay link stays as it is: it protects this process, and a single
// larger message still ends the link.
const (
	// uploadPartMax is the most body bytes one part may carry.
	uploadPartMax = 24 << 10
	// maxUploadBody is the most a reassembled request may hold.
	maxUploadBody = 16 << 20
	// uploadIdle is how long an unfinished upload waits for its next part.
	uploadIdle = 30 * time.Second
)

var (
	errUploadOrder   = errors.New("upload part is out of order")
	errUploadTooBig  = errors.New("upload is larger than the limit")
	errUploadInvalid = errors.New("upload part is invalid")
	errUploadExpired = errors.New("upload timed out waiting for its next part")
)

// upload is one request being received. A session holds at most one: the page
// sends one request's parts before the next request's, and a part of anything
// else abandons it.
type upload struct {
	id, method, path string
	total, next      int
	body             []byte
	touched          time.Time
}

// uploads is a controller session's single in-flight upload.
type uploads struct {
	current *upload
}

// add takes one part. A finished upload comes back with ok true and is no longer
// held; any refusal also drops the upload it concerned, so a page that was told
// no never leaves bytes behind. Nothing is sized from what the page declares:
// the bytes held are the bytes that arrived, counted as they arrive, and the
// declared count only has to be one the limit could ever allow.
func (u *uploads) add(command controllerCommand, now time.Time) (request DesktopRequest, ok bool, err error) {
	part, decodeErr := base64.RawURLEncoding.DecodeString(command.Body)
	if command.Total < 1 || command.Total > maxUploadBody/uploadPartMax || command.Index < 0 || command.Index >= command.Total ||
		decodeErr != nil || len(part) == 0 || len(part) > uploadPartMax {
		u.drop(command.ID)
		return DesktopRequest{}, false, errUploadInvalid
	}
	if command.Index == 0 {
		if strings.TrimSpace(command.Method) == "" || strings.TrimSpace(command.Path) == "" {
			u.drop(command.ID)
			return DesktopRequest{}, false, errUploadInvalid
		}
		u.current = &upload{id: command.ID, method: command.Method, path: command.Path, total: command.Total, touched: now}
	}
	cur := u.current
	switch {
	case cur == nil || cur.id != command.ID:
		return DesktopRequest{}, false, errUploadOrder
	case now.Sub(cur.touched) >= uploadIdle:
		u.current = nil
		return DesktopRequest{}, false, errUploadExpired
	case command.Index != cur.next || command.Total != cur.total:
		u.current = nil
		return DesktopRequest{}, false, errUploadOrder
	case len(cur.body)+len(part) > maxUploadBody:
		u.current = nil
		return DesktopRequest{}, false, fmt.Errorf("%w (%d MiB)", errUploadTooBig, maxUploadBody>>20)
	}
	cur.body = append(cur.body, part...)
	cur.next++
	cur.touched = now
	if cur.next < cur.total {
		return DesktopRequest{}, false, nil
	}
	u.current = nil
	return DesktopRequest{Method: cur.method, Path: cur.path, Body: cur.body}, true, nil
}

// expire frees an upload nothing has added to for too long. It runs on every
// command the session receives, so held bytes do not wait for the next part of
// the same upload to be noticed; there is no timer to stop when the link ends.
func (u *uploads) expire(now time.Time) {
	if u.current != nil && now.Sub(u.current.touched) >= uploadIdle {
		u.current = nil
	}
}

func (u *uploads) drop(id string) {
	if u.current != nil && u.current.id == id {
		u.current = nil
	}
}
