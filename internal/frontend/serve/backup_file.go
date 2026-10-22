package serve

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"reasonix/internal/ext/configbackup"
)

// A backup file is the account backup's sealed envelope, saved by the person.
// These routes serve the window only: the file can hold keys, so a paired
// device (and the relay a phone arrives by) gets nothing here. The kernel never
// writes the file; the window saves and opens it, so no plaintext touches disk.
func (s *Server) registerBackupFileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /backups/file", s.backupFileCatalog)
	mux.HandleFunc("POST /backups/file/export", s.exportBackupFile)
	mux.HandleFunc("POST /backups/file/preview", s.previewBackupFile)
	mux.HandleFunc("POST /backups/file/apply", s.applyBackupFile)
}

const (
	// A sealed envelope is at most MaxEnvelopeBytes; base64 adds a third.
	maxBackupFileBody   = configbackup.MaxEnvelopeBytes/3*4 + 64<<10
	maxBackupExportBody = 64 << 10
)

// kdf holds one Argon2id derivation at a time: each takes 64 MiB, and a loop of
// guesses at the import route must not be able to stack them.
var backupKDF sync.Mutex

func hostOnlyBackupFile(w http.ResponseWriter, r *http.Request) bool {
	if hostReach(r) {
		return true
	}
	refuse(w, http.StatusNotFound, codeDeviceHostOnly, "a paired device cannot use this", nil)
	return false
}

// backupFileCatalog is what the export form needs and a signed-out window has no
// other way to ask: the categories and the shortest passphrase.
func (s *Server) backupFileCatalog(w http.ResponseWriter, r *http.Request) {
	if !hostOnlyBackupFile(w, r) {
		return
	}
	writeJSON(w, map[string]any{"categories": configbackup.Categories(), "minPassphrase": configbackup.MinPassphraseRunes})
}

type exportBackupFileRequest struct {
	Categories []string `json:"categories"`
	Passphrase string   `json:"passphrase"`
	AppVersion string   `json:"appVersion"`
}

func (s *Server) exportBackupFile(w http.ResponseWriter, r *http.Request) {
	if !hostOnlyBackupFile(w, r) {
		return
	}
	var req exportBackupFileRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBackupExportBody)).Decode(&req); err != nil {
		badBody(w)
		return
	}
	cats, err := configbackup.ParseCategories(req.Categories)
	if err != nil {
		refuseBackup(w, err)
		return
	}
	snap, err := configbackup.Collect(configbackup.CollectOptions{Categories: cats, AppVersion: strings.TrimSpace(req.AppVersion)})
	if err != nil {
		saveFailed(w, http.StatusInternalServerError, "backup.collect_failed", err)
		return
	}
	backupKDF.Lock()
	envelope, err := configbackup.Seal(snap, req.Passphrase)
	backupKDF.Unlock()
	if err != nil {
		refuseBackup(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"name":    "reasonix-config-" + time.Now().Format("20060102") + ".rxcb",
		"file":    base64.StdEncoding.EncodeToString(envelope),
		"omitted": snap.Omitted,
	})
}

func (s *Server) previewBackupFile(w http.ResponseWriter, r *http.Request) {
	if !hostOnlyBackupFile(w, r) {
		return
	}
	var req struct {
		File       string `json:"file"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBackupFileBody)).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			refuseBackup(w, configbackup.ErrTooLarge)
			return
		}
		badBody(w)
		return
	}
	envelope, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.File))
	if err != nil {
		refuseBackup(w, configbackup.ErrMalformed)
		return
	}
	backupKDF.Lock()
	snap, err := configbackup.Open(envelope, req.Passphrase)
	backupKDF.Unlock()
	if err != nil {
		refuseBackup(w, err)
		return
	}
	plan, err := backupPlanner.Preview(snap)
	if err != nil {
		saveFailed(w, http.StatusInternalServerError, "backup.preview_failed", err)
		return
	}
	writeJSON(w, plan)
}

func (s *Server) applyBackupFile(w http.ResponseWriter, r *http.Request) {
	if !hostOnlyBackupFile(w, r) {
		return
	}
	s.applyBackupPlan(w, r)
}
