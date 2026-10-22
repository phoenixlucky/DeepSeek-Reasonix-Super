package boot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

type traceCollector struct {
	*httptest.Server
	mu    sync.Mutex
	posts []string
	auth  []string
}

func newTraceCollector(t *testing.T) *traceCollector {
	t.Helper()
	c := &traceCollector{}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.posts = append(c.posts, r.URL.Path+" "+string(b))
		c.auth = append(c.auth, r.Header.Get("Authorization"))
		c.mu.Unlock()
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *traceCollector) all() (string, []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.posts, "\n"), append([]string(nil), c.auth...)
}

var traceRuns int

type traceProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *traceProvider) Name() string { return "boot-trace" }

func (p *traceProvider) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.calls++
	first := p.calls == 1
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 4)
	if first {
		args, _ := json.Marshal(map[string]string{"path": "CANARY-notes.txt"})
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "call-1", Name: "read_file", Arguments: string(args)}}
		ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: 900, CompletionTokens: 40, CacheHitTokens: 700, CacheMissTokens: 200, TotalTokens: 940, FinishReason: "tool_calls"}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "CANARY-ANSWER"}
		ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: 950, CompletionTokens: 10, TotalTokens: 960, FinishReason: "stop"}}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// runTraceTurn builds a real controller, runs one turn that reads a file, and
// closes it so the exporter flushes.
func runTraceTurn(t *testing.T, userConfig, projectConfig string) {
	t.Helper()
	home := isolateConfigHome(t)
	reasonixHome := filepath.Join(home, ".reasonix")
	t.Setenv("REASONIX_HOME", reasonixHome)
	t.Setenv("OTEL_TEST_AUTH", "Bearer TRACE-SECRET")
	workspace := robustTempDir(t)
	t.Chdir(workspace)
	writeFile(t, workspace, "CANARY-notes.txt", "CANARY-FILE-CONTENT")
	traceRuns++
	kind := fmt.Sprintf("boot-trace-%d", traceRuns)
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return &traceProvider{}, nil })
	writeFile(t, workspace, "reasonix.toml", fmt.Sprintf(`
default_model = "test-model"
[agent]
system_prompt = "BASE"
[codegraph]
enabled = false
[[providers]]
name = "test-model"
kind = %q
model = "x"
%s`, kind, projectConfig))
	if userConfig != "" {
		writeFile(t, reasonixHome, "config.toml", userConfig)
	}
	approveWorkspace(t, workspace)
	ctrl, err := Build(t.Context(), Options{Sink: event.Discard, Home: reasonixHome, WorkspaceRoot: workspace, Version: "9.9.9"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := ctrl.Run(ctx, "CANARY-PROMPT summarise the notes"); err != nil {
		t.Fatal(err)
	}
	ctrl.Close()
}

// A turn through the real assembly reaches the collector as one trace with the
// agent, model and tool spans, authenticated by the user's variable, and
// nothing the person or the model wrote appears anywhere in the payload.
func TestEffectTraceExportReachesTheCollectorAsStructureOnly(t *testing.T) {
	c := newTraceCollector(t)
	runTraceTurn(t, fmt.Sprintf("[observability.otlp]\nenabled = true\nendpoint = %q\nheader_name = \"Authorization\"\nheader_env = \"OTEL_TEST_AUTH\"\nservice_name = \"effect\"\n", c.URL), "")
	body, auth := c.all()
	if body == "" {
		t.Fatal("the collector received nothing")
	}
	for _, want := range []string{"/v1/traces ", `"invoke_agent reasonix"`, `"execute_tool read_file"`, `"chat `, `"gen_ai.usage.input_tokens"`, `"900"`, `"service.name"`, `"effect"`, `"9.9.9"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("payload lacks %s:\n%s", want, body)
		}
	}
	for _, leak := range []string{"CANARY", "TRACE-SECRET", "summarise"} {
		if strings.Contains(body, leak) {
			t.Fatalf("payload carries %q", leak)
		}
	}
	if len(auth) == 0 || auth[0] != "Bearer TRACE-SECRET" {
		t.Fatalf("collector saw authorization %q", auth)
	}
}

func TestEffectTraceExportSendsNothingUnlessTheUserTurnedItOn(t *testing.T) {
	c := newTraceCollector(t)
	runTraceTurn(t, "", "")
	if body, _ := c.all(); body != "" {
		t.Fatalf("default config exported: %s", body)
	}
	runTraceTurn(t, fmt.Sprintf("[observability.otlp]\nendpoint = %q\n", c.URL), "")
	if body, _ := c.all(); body != "" {
		t.Fatal("an endpoint without the switch exported")
	}
}

// A cloned repository's reasonix.toml cannot start an export or redirect one.
func TestEffectTraceExportCannotBeStartedOrRedirectedByTheProjectFile(t *testing.T) {
	attacker, mine := newTraceCollector(t), newTraceCollector(t)
	project := fmt.Sprintf("[observability.otlp]\nenabled = true\nendpoint = %q\n", attacker.URL)
	runTraceTurn(t, "", project)
	if body, _ := attacker.all(); body != "" {
		t.Fatal("the project file started an export")
	}
	runTraceTurn(t, fmt.Sprintf("[observability.otlp]\nenabled = true\nendpoint = %q\n", mine.URL), project)
	if body, _ := attacker.all(); body != "" {
		t.Fatal("the project file redirected the user's export")
	}
	if body, _ := mine.all(); body == "" {
		t.Fatal("the user's own export did not arrive")
	}
}
