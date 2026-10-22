package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/session/control"
	"reasonix/internal/tools/jobs"
)

type jobPeeker struct {
	control.SessionAPI
	peek  jobs.Peek
	known bool
	asked []int
}

func (c *jobPeeker) JobPeek(_ string, max int) (jobs.Peek, bool) {
	c.asked = append(c.asked, max)
	return c.peek, c.known
}

func getJobOutput(t *testing.T, ctrl *jobPeeker, target string) (int, JobOutput, Reason) {
	t.Helper()
	s := &Server{ctrl: ctrl}
	mux := http.NewServeMux()
	s.registerJobRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	var out JobOutput
	var reason Reason
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	_ = json.Unmarshal(rec.Body.Bytes(), &reason)
	return rec.Code, out, reason
}

func TestAJobsOutputIsShownWithItsStatusAndHowMuchWasCut(t *testing.T) {
	ctrl := &jobPeeker{known: true, peek: jobs.Peek{Text: "tail", Status: jobs.Running, Bytes: 9000, Truncated: true}}
	code, out, _ := getJobOutput(t, ctrl, "/jobs/job-1/output")
	if code != http.StatusOK || out.Text != "tail" || out.Status != "running" || out.Bytes != 9000 || !out.Truncated {
		t.Fatalf("status %d body %+v", code, out)
	}
}

func TestTheAmountAskedForIsBoundedByTheServer(t *testing.T) {
	for target, want := range map[string]int{
		"/jobs/j/output":              jobOutputDefault,
		"/jobs/j/output?max=2048":     2048,
		"/jobs/j/output?max=99999999": jobOutputMax,
		"/jobs/j/output?max=-5":       jobOutputDefault,
		"/jobs/j/output?max=abc":      jobOutputDefault,
	} {
		ctrl := &jobPeeker{known: true}
		getJobOutput(t, ctrl, target)
		if len(ctrl.asked) != 1 || ctrl.asked[0] != want {
			t.Errorf("%s asked for %v, want %d", target, ctrl.asked, want)
		}
	}
}

func TestAJobThatIsGoneIsRefusedByCode(t *testing.T) {
	code, _, reason := getJobOutput(t, &jobPeeker{known: false}, "/jobs/job-1/output")
	if code != http.StatusConflict || reason.Code != "job.not_running" {
		t.Fatalf("status %d code %q, want 409 job.not_running", code, reason.Code)
	}
}
