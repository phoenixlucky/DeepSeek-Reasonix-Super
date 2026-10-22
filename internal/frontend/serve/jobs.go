package serve

import (
	"net/http"
	"strconv"
)

func (s *Server) registerJobRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /jobs/{id}/cancel", s.cancelJob)
	mux.HandleFunc("GET /jobs/{id}/output", s.jobOutput)
}

// cancelJob stops one of this session's background jobs. A job that already
// ended, or belongs to another session, is refused rather than acknowledged:
// the panel is showing a row that no longer answers to it.
func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	ctl, id := s.ctl(), r.PathValue("id")
	if !ctl.CancelJob(id) {
		// A job the kill was already asked for stays on the running list until its
		// process tree has gone, so "listed, and the kill refuses" is that wait.
		for _, j := range ctl.Jobs() {
			if j.ID == id {
				refuse(w, http.StatusConflict, "job.stop_in_progress", "the stop was already requested; the job is still shutting down", nil)
				return
			}
		}
		refuse(w, http.StatusConflict, "job.not_running", "no running background job with this id in this session", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// JobOutput is the end of a running background job's output as the panel shows
// it: the text, how much the job has written in all, and whether the text is
// only the last part of that.
type JobOutput struct {
	Text      string `json:"text"`
	Status    string `json:"status"`
	Bytes     int64  `json:"bytes"`
	Truncated bool   `json:"truncated"`
}

const (
	jobOutputDefault = 16 << 10
	jobOutputMax     = 64 << 10
)

// jobOutput lets the person read what a background job has printed. It is a
// look, not a read: the model's own reader keeps everything it has not seen.
func (s *Server) jobOutput(w http.ResponseWriter, r *http.Request) {
	limit := jobOutputDefault
	if raw := r.URL.Query().Get("max"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = min(n, jobOutputMax)
		}
	}
	peek, ok := s.ctl().JobPeek(r.PathValue("id"), limit)
	if !ok {
		refuse(w, http.StatusConflict, "job.not_running", "no background job with this id in this session", nil)
		return
	}
	writeJSON(w, JobOutput{Text: peek.Text, Status: string(peek.Status), Bytes: peek.Bytes, Truncated: peek.Truncated})
}
