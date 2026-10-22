package shellsafe

import "testing"

func TestGhReadOnlyCalls(t *testing.T) {
	reads := []string{
		"gh pr list", "gh pr list --state open --json number,title", "gh pr view 12", "gh pr view 12 --comments",
		"gh pr status", "gh pr checks 12", "gh pr checks 12 --watch", "gh pr diff 12", "gh pr diff 12 --patch",
		"gh issue list -L 30", "gh issue view 7", "gh issue status",
		"gh run list --limit 5", "gh run view 99 --log-failed", "gh release list", "gh release view v1.0.0",
		"gh repo view", "gh repo view owner/name", "gh repo list owner", "gh auth status",
		"gh api repos/o/r/pulls/1", "gh api repos/o/r/pulls --paginate --jq '.[].number'",
		"gh api -H 'Accept: application/vnd.github+json' repos/o/r", "gh api repos/o/r -q .name", "gh api --jq=.name repos/o/r",
		"gh api user -i", "gh api repos/o/r --template '{{.name}}'",
	}
	for _, command := range reads {
		if _, _, _, ok := ClassifyReadOnlyCommand(command); !ok {
			t.Errorf("%q must be a read", command)
		}
	}
	escapes := []string{
		"gh pr view 12 --web", "gh pr view 12 -w", "gh pr list -sw open", "gh issue view 7 --web=true", "gh repo view --web", "gh run view 9 -w",
		"gh auth status --show-token", "gh auth status -t", "gh auth status -ht github.com",
		"gh pr create", "gh pr merge 12", "gh pr checkout 12", "gh pr close 12", "gh pr comment 12 -b x", "gh pr edit 12",
		"gh issue create", "gh issue close 7", "gh run download 99", "gh run cancel 99", "gh run delete 99", "gh run rerun 99", "gh run watch 99",
		"gh release download v1", "gh release create v2", "gh release delete v1",
		"gh repo clone o/r", "gh repo fork", "gh repo create x", "gh repo delete x", "gh repo edit",
		"gh extension install o/x", "gh alias set a 'pr list'", "gh config set editor vim", "gh codespace ssh", "gh variable set X", "gh workflow run w.yml",
		"gh auth login", "gh auth logout", "gh auth token", "gh auth setup-git",
		"gh api repos/o/r -X DELETE", "gh api repos/o/r --method=PATCH", "gh api repos/o/r -XPOST", "gh api repos/o/r -f a=b", "gh api repos/o/r -F a=@file",
		"gh api repos/o/r --field a=b", "gh api repos/o/r --raw-field a=b", "gh api repos/o/r --input body.json", "gh api graphql -f query=x",
		"gh api graphql", "gh api https://evil.test/x", "gh api repos/o/r --hostname evil.test", "gh api repos/o/r -H 'X-HTTP-Method-Override: DELETE'",
		"gh api", "gh api a b", "gh api repos/o/r -Xdelete", "gh api repos/o/r --paginate=false", "gh api repos/o/r --unknown",
		"gh api user --jq env", "gh api user --jq 'env|keys'", "gh api user --jq '$ENV'", "gh api user -q '$__loc__ | env'", "gh api user --jq=env", "gh api user -qenv",
		"gh pr view 1 --json number --jq '$ENV'", "gh pr view 1 --json number -q env", "gh pr list --jq 'input'", "gh pr list --jq 'debug'",
		"gh issue list --json number --jq 'stderr'", "gh run view 9 --json status --jq halt_error", "gh pr list --jq=ENV", "gh pr list -sq env",
		"gh api user --template '{{.login}}\n{{env}}'", "gh api user --template env", "gh pr view 1 --json environment --jq .environment",
		"gh api user --jq 'import \"a\" as a; .'", "gh api user --jq 'include \"a\"; .'", "gh pr list --jq get_search_list", "gh pr list --jq '$__prog_args'", "gh api user -q modulemeta",
		"gh api user -H $'X: a\nb'", "gh api user --cache $'1h\r'",
		"gh", "gh pr", "gh pr unknown", "gh -R o/r pr view 1", "gh status",
	}
	for _, command := range escapes {
		if base, sub, _, ok := ClassifyReadOnlyCommand(command); ok {
			t.Errorf("%q must not be classified as a read (got %s %s)", command, base, sub)
		}
	}
}
