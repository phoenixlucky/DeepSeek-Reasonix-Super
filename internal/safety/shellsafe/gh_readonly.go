package shellsafe

import "strings"

// ghReadVerbs lists the GitHub CLI calls that only read. gh is three words deep
// (`gh pr view`), so the two-word ReadOnlyPrefixes table cannot hold it, and a
// call outside this list is simply not classified rather than classified as a
// write: what a `gh pr create` does to the remote says nothing about the tree.
var ghReadVerbs = map[string]map[string]bool{
	"pr":      {"list": true, "view": true, "status": true, "checks": true, "diff": true},
	"issue":   {"list": true, "view": true, "status": true},
	"run":     {"list": true, "view": true},
	"release": {"list": true, "view": true},
	"repo":    {"list": true, "view": true},
	"auth":    {"status": true},
}

// ghAPIFlags are the only options a read-only `gh api` call may carry; the
// valued ones consume the next word. Anything else, including every spelling of
// a method, field, input file or hostname, is outside the list.
var ghAPIFlags = map[string]bool{
	"--paginate": false, "--slurp": false, "-i": false, "--include": false,
	"--silent": false, "--verbose": false,
	"-H": true, "--header": true, "-q": true, "--jq": true,
	"-t": true, "--template": true, "--cache": true, "-p": true, "--preview": true,
}

// ghReadOnlyCall reports whether fields are a `gh` call that only reads, and
// the group word that serves as its subcommand.
func ghReadOnlyCall(fields []string) (sub string, ok bool) {
	if len(fields) < 3 || strings.ToLower(fields[0]) != "gh" || hasResolvedSubstitution(fields) {
		return "", false
	}
	if ghFilterReachesHost(fields[2:]) {
		return "", false
	}
	group := strings.ToLower(fields[1])
	if group == "api" {
		return group, ghAPIReadsOnly(fields[2:])
	}
	if !ghReadVerbs[group][strings.ToLower(fields[2])] {
		return "", false
	}
	return group, !ghOpensOrRevealsSecret(group, fields[3:])
}

// ghOpensOrRevealsSecret reports options that make an otherwise read call do
// something else: --web starts the browser, --show-token prints the credential.
// A short flag anywhere in a cluster counts, as getopt reads it.
func ghOpensOrRevealsSecret(group string, args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--web" || strings.HasPrefix(arg, "--web=") {
			return true
		}
		if group == "auth" && (arg == "--show-token" || strings.HasPrefix(arg, "--show-token=")) {
			return true
		}
	}
	return hasShortFlag(args, 'w') || (group == "auth" && hasShortFlag(args, 't'))
}

// ghAPIReadsOnly accepts `gh api ENDPOINT` with only the listed options. gh
// turns any field or input into a POST and --method into whatever it names, and
// sends its credential to --hostname or a full URL, so none of those may appear.
func ghAPIReadsOnly(args []string) bool {
	endpoints := 0
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			if endpoints++; endpoints > 1 || strings.Contains(arg, "://") || strings.EqualFold(arg, "graphql") {
				return false
			}
			continue
		}
		name, _, attached := strings.Cut(arg, "=")
		valued, listed := ghAPIFlags[name]
		if !listed || (!valued && attached) {
			return false
		}
		value := ""
		if valued && attached {
			_, value, _ = strings.Cut(arg, "=")
		} else if valued {
			if i+1 >= len(args) {
				return false
			}
			i++
			value = args[i]
		}
		if (name == "-H" || name == "--header") && strings.Contains(strings.ToLower(value), "override") {
			return false
		}
	}
	return endpoints == 1
}

// ghHostReaching are the words through which a gh jq filter reaches outside its
// input: the environment (`env`, `$ENV`), other streams (`input`, `debug`,
// `stderr`, `halt_error`) and the module search path (`import`, `include`). A
// substring match, so a field named "environment" costs an approval, not a miss.
var ghHostReaching = []string{"env", "input", "debug", "stderr", "halt_error", "import", "include", "get_search_list", "prog_args", "modulemeta"}

// ghFilterReachesHost reports a call that carries a jq or template filter whose
// text could reach the host, or any word with a control character. Every spelling
// of the filter options counts, attached or clustered, because cobra reads the
// rest of a short cluster as the value.
func ghFilterReachesHost(args []string) bool {
	filtered := hasShortFlag(args, 'q') || hasShortFlag(args, 't')
	for _, arg := range args {
		if strings.IndexFunc(arg, func(r rune) bool { return r < 0x20 && r != '\t' || r == 0x7f }) >= 0 {
			return true
		}
		if arg == "--jq" || arg == "--template" || strings.HasPrefix(arg, "--jq=") || strings.HasPrefix(arg, "--template=") {
			filtered = true
		}
	}
	if !filtered {
		return false
	}
	for _, arg := range args {
		lower := strings.ToLower(arg)
		for _, word := range ghHostReaching {
			if strings.Contains(lower, word) {
				return true
			}
		}
	}
	return false
}
