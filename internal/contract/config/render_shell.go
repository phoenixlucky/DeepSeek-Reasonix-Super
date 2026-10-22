package config

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

func renderToolsShell(b *strings.Builder, s ShellConfig, scope RenderScope) {
	b.WriteString("[tools.shell]\n")
	b.WriteString("# The interpreter the bash tool runs commands in; your login shell does not change it.\n")
	if s.Prefer != "" {
		fmt.Fprintf(b, "prefer = %q   # auto|bash|powershell|pwsh; empty/default = auto-detect\n", s.Prefer)
	} else {
		b.WriteString("# prefer = \"auto\"   # auto|bash|powershell|pwsh; empty/default = auto-detect\n")
	}
	if s.Path != "" {
		fmt.Fprintf(b, "path   = %q   # absolute path to the shell executable; empty = PATH lookup\n", s.Path)
	} else {
		b.WriteString("# path   = \"/opt/homebrew/bin/bash\"   # absolute path to the shell executable; empty = PATH lookup\n")
	}
	if scope != RenderScopeProject {
		renderShellArgs(b, s.Args)
	}
	b.WriteString("\n")
	if scope != RenderScopeProject {
		renderShellEnv(b, s.Env)
	}
}

// renderShellArgs writes launcher arguments for [tools.shell] path. They are
// the user's alone, so a project file never carries them.
func renderShellArgs(b *strings.Builder, args []string) {
	if len(args) == 0 {
		b.WriteString("# args   = [\"-d\", \"Ubuntu\", \"--\", \"/bin/bash\"]   # Windows: launcher arguments for path (wsl.exe); verified by running them\n")
		return
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = strconv.Quote(a)
	}
	fmt.Fprintf(b, "args   = [%s]   # launcher arguments for path, run as written (no shell)\n", strings.Join(quoted, ", "))
}

// renderShellEnv writes the [tools.shell.env] sub-table. The full template shows
// a commented example when nothing is set, so the option is discoverable.
func renderShellEnv(b *strings.Builder, env map[string]string) {
	if len(env) == 0 {
		b.WriteString("# [tools.shell.env]   # extra variables every bash command inherits\n")
		b.WriteString("# BASH_ENV = \"~/.reasonix/bashsandbox.rc\"\n\n")
		return
	}
	b.WriteString("[tools.shell.env]\n")
	for _, k := range slices.Sorted(maps.Keys(env)) {
		fmt.Fprintf(b, "%s = %q\n", renderTOMLKeyPart(k), env[k])
	}
	b.WriteString("\n")
}
