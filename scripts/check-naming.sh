#!/usr/bin/env bash
#
# The project was renamed from MediGo to MediKube. This gate keeps the old name
# from coming back through a copied snippet, a stale template or a long-lived
# branch.
#
# It runs over tracked files *and* untracked ones git has not been told to
# ignore. The tracked-only sweep it replaces read the index, so a branch that
# added sixty files and had not committed them yet passed this gate having
# scanned none of them — a green tick that meant nothing at exactly the moment
# it mattered most. Ignored files stay out: they are build products and tool
# binaries, and neither ships.
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

# NUL-delimited, so a path with a space in it is one path. This script has to
# name the forbidden string, so it excludes itself — and that is the only
# exclusion there is. An allowlist for prose is how the name comes back.
files=()
while IFS= read -r -d '' path; do
	if [ "$path" = "$self" ]; then
		continue
	fi
	files+=("$path")
done < <(
	git ls-files -z
	git ls-files -z --others --exclude-standard
)

if [ ${#files[@]} -eq 0 ]; then
	fail "No files to scan — the sweep would pass over an empty repository."
	exit "$status"
fi

# -I skips binaries; the separator is optional so medi-go and medi_go are caught
# too. -H forces the filename even when the list happens to be one file.
if hits=$(grep -HnIiE 'medi[-_]?go' -- "${files[@]}"); then
	fail "The old project name is back. Use MediKube / medikube / MEDIKUBE:"
	printf '%s\n' "$hits" >&2
fi

if named=$(printf '%s\n' "${files[@]}" | grep -iE 'medi[-_]?go'); then
	fail "These paths are still named after the old project:"
	printf '%s\n' "$named" >&2
fi

# Without this the gate would pass on a repository that had simply deleted
# every mention of the project rather than renamed it.
if ! grep -qI 'MediKube' -- "${files[@]}"; then
	fail "No occurrence of 'MediKube' anywhere — the sweep above passed vacuously."
fi

# The published image is the one place outside the prose where the name is
# load-bearing, so it is asserted by hand. <project> is the placeholder the
# house-patterns dossier uses when describing the shared build workflow.
#
# Two stages rather than one negative lookahead: a lookahead needs PCRE, which
# BSD grep does not have, and matching each reference and then filtering the
# permitted ones judges every occurrence on a line rather than the line.
references=$(grep -HnoIE 'ghcr\.io/windkube/[A-Za-z0-9_<>-]*' -- "${files[@]}" || true)
if bad=$(printf '%s' "$references" | grep -vE ':ghcr\.io/windkube/(medikube|<project>)$'); then
	fail "Image references must be ghcr.io/windkube/medikube:"
	printf '%s\n' "$bad" >&2
fi

if [ "$status" -eq 0 ]; then
	echo "ok    no MediGo references, and MediKube is in use"
fi
exit "$status"
