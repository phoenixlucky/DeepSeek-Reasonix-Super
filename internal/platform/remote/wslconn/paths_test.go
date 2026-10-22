package wslconn

import (
	"context"
	"errors"
	"io"
	"testing"

	"reasonix/internal/platform/wsl"
)

// answers is a distribution whose wslpath says what the table says.
func answers(table map[string]wsl.Result) *Conn {
	return &Conn{distro: "Ubuntu", run: func(_ context.Context, _ io.Reader, args ...string) (wsl.Result, error) {
		// -d Ubuntu --exec wslpath <flag> <path>
		return table[args[5]], nil
	}}
}

func TestATranslationIsTheDistributionsAnswerOrARefusal(t *testing.T) {
	c := answers(map[string]wsl.Result{
		`C:\work\app`: {Stdout: []byte("/mnt/c/work/app\n")},
		`Z:\none`:     {ExitCode: 1},
		`C:\odd`:      {Stdout: []byte("C:\\odd\n")},
		"/mnt/c/work": {Stdout: []byte("C:\\work\r\n")},
		"/home/dev":   {Stdout: []byte("\\\\wsl.localhost\\Ubuntu\\home\\dev\n")},
		"/odd":        {Stdout: []byte("/odd\n")},
	})
	ctx := context.Background()
	if got, err := c.ToLinux(ctx, `C:\work\app`); err != nil || got != "/mnt/c/work/app" {
		t.Fatalf("ToLinux = %q, %v", got, err)
	}
	for _, in := range []string{`Z:\none`, `C:\odd`, `/home/dev`, `rel`, ``} {
		if _, err := c.ToLinux(ctx, in); !errors.Is(err, ErrUntranslatable) {
			t.Errorf("ToLinux(%q) = %v, want ErrUntranslatable", in, err)
		}
	}
	if got, err := c.ToWindows(ctx, "/mnt/c/work"); err != nil || got != `C:\work` {
		t.Fatalf("ToWindows = %q, %v", got, err)
	}
	if got, err := c.ToWindows(ctx, "/home/dev"); err != nil || got != `\\wsl.localhost\Ubuntu\home\dev` {
		t.Fatalf("ToWindows share = %q, %v", got, err)
	}
	for _, in := range []string{"/odd", "rel", `C:\x`} {
		if _, err := c.ToWindows(ctx, in); !errors.Is(err, ErrUntranslatable) {
			t.Errorf("ToWindows(%q) = %v, want ErrUntranslatable", in, err)
		}
	}
}
