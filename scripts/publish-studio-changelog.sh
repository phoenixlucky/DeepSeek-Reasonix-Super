#!/usr/bin/env bash
set -euo pipefail

: "${VERSION:?}"
version="${VERSION#v}"
if [[ ! "$version" =~ ^2\.[0-9]+\.[0-9]+$ ]]; then
  echo "::error::website changelog publication requires a stable Studio 2.x version" >&2
  exit 1
fi
repo="${GITHUB_REPOSITORY:?}"
request="studio-${GITHUB_RUN_ID:?}-${GITHUB_RUN_ATTEMPT:?}"
gh workflow run pages.yml --repo "$repo" --ref website \
  -f "release_version=$version" -f "studio_version=$version" -f "release_request=$request"

for ((attempt = 0; attempt < 12; attempt++)); do
  run_id="$(gh api "repos/$repo/actions/workflows/pages.yml/runs?event=workflow_dispatch&branch=website&per_page=100" \
    --jq ".workflow_runs[] | select(.display_title | endswith(\" [$request]\")) | .id" | head -n 1)"
  if [ -n "$run_id" ]; then
    echo "Website deployment: https://github.com/$repo/actions/runs/$run_id"
    gh run watch "$run_id" --repo "$repo" --exit-status --interval 10
    exit 0
  fi
  sleep 5
done
echo "::error::website deployment for $request was not found" >&2
exit 1
