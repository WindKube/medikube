#!/usr/bin/env bash
#
# The project was renamed from MediGo to MediKube. This gate keeps the old name
# from coming back through a copied snippet, a stale template or a long-lived
# branch. It runs over tracked files only, so an untracked scratch file never
# fails a build.
#
# MediKeep is deliberately not guarded. It names the predecessor product this
# project reimagines, and the research corpus contrasts the two on purpose;
# medikeep-mcp/ and medi-keep-go/ are real sibling directories in the monorepo.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

status=0
self=scripts/check-naming.sh

fail() {
	status=1
	printf '\n%s\n' "$1" >&2
}

# The separator is optional so medi-go and medi_go are caught too. This script
# has to name the forbidden string, so it excludes itself from the sweep.
if hits=$(git grep -nIiE 'medi[-_]?go' -- . ":(exclude)$self"); then
	fail "The old project name is back. Use MediKube / medikube / MEDIKUBE:"
	printf '%s\n' "$hits" >&2
fi

paths=$(git ls-files | grep -iE 'medi[-_]?go' || true)
if [ -n "$paths" ]; then
	fail "These tracked paths are still named after the old project:"
	printf '%s\n' "$paths" >&2
fi

# Without this the gate would pass on a repository that had simply deleted
# every mention of the project rather than renamed it.
if ! git grep -qI 'MediKube' -- .; then
	fail "No occurrence of 'MediKube' anywhere — the sweep above passed vacuously."
fi

# The published image is the one place outside the prose where the name is
# load-bearing, so it is asserted by hand. <project> is the placeholder the
# house-patterns dossier uses when describing the shared build workflow.
if bad=$(git grep -nIP 'ghcr\.io/windkube/(?!medikube|<project>)' -- .); then
	fail "Image references must be ghcr.io/windkube/medikube:"
	printf '%s\n' "$bad" >&2
fi

if [ "$status" -eq 0 ]; then
	echo "ok    no MediGo references, and MediKube is in use"
fi
exit "$status"
