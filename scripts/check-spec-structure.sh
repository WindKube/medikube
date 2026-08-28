#!/usr/bin/env bash
#
# The "test" tier for a repository that is only specifications. There is no
# compiler to catch a half-written feature directory, a contract folder with no
# index, or a cross-reference to a file somebody renamed, so these invariants
# are asserted here instead. Every assertion below holds on the tree as it
# stands; a new one belongs here only once it is already true.
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

failures=0

fail() {
	printf 'FAIL  %s\n' "$*" >&2
	failures=$((failures + 1))
}

ok() {
	printf 'ok    %s\n' "$*"
}

# --- feature directories -------------------------------------------------
# speckit emits one directory per feature and every downstream skill assumes
# the full set of artefacts is present. A missing plan.md fails silently at
# authoring time and loudly six weeks later.
required_artefacts=(spec.md plan.md tasks.md research.md data-model.md quickstart.md)

feature_dirs=()
while IFS= read -r dir; do
	feature_dirs+=("$dir")
done < <(find specs -mindepth 1 -maxdepth 1 -type d -name '[0-9][0-9][0-9]-*' | sort)

if [ ${#feature_dirs[@]} -eq 0 ]; then
	fail "specs/ contains no NNN-* feature directory"
else
	ok "found ${#feature_dirs[@]} feature directories under specs/"
fi

for dir in "${feature_dirs[@]}"; do
	name="${dir#specs/}"
	if ! printf '%s' "$name" | grep -qE '^[0-9]{3}-[a-z0-9]+(-[a-z0-9]+)*$'; then
		fail "$dir: directory name is not NNN-kebab-case"
	fi

	for artefact in "${required_artefacts[@]}"; do
		[ -f "$dir/$artefact" ] || fail "$dir: missing $artefact"
	done

	# Every feature carries the requirements checklist the /speckit.checklist
	# step produces; it is the record that the spec was reviewed at all.
	[ -f "$dir/checklists/requirements.md" ] || fail "$dir: missing checklists/requirements.md"
done

# --- contract folders ----------------------------------------------------
# A contracts/ directory is a set of sibling documents with no natural entry
# point, so the README is the index that makes it navigable.
while IFS= read -r dir; do
	[ -f "$dir/README.md" ] || fail "$dir: missing README.md"
done < <(find specs -mindepth 2 -type d -name contracts | sort)

# --- constitution --------------------------------------------------------
constitution=.specify/memory/constitution.md
if [ ! -f "$constitution" ]; then
	fail "$constitution is missing"
elif ! version="$(grep -oE '\*\*Version\*\*: [0-9]+\.[0-9]+\.[0-9]+' "$constitution" | head -1)"; then
	fail "$constitution: no parseable '**Version**: MAJOR.MINOR.PATCH' line"
else
	ok "constitution ${version##*: }"
fi

# --- unresolved template markers -----------------------------------------
# The speckit templates ship these and the corpus is expected to have replaced
# every one. Note the colon: the corpus legitimately writes a bare
# "[NEEDS CLARIFICATION]" in prose that asserts none remain, and the marker
# proper is "[NEEDS CLARIFICATION: <question>]".
markers=('[NEEDS CLARIFICATION:' '[PLACEHOLDER' 'TODO(')
for marker in "${markers[@]}"; do
	if hits="$(grep -rnF --include='*.md' -e "$marker" specs "$constitution")"; then
		fail "unresolved template marker '$marker':"
		printf '%s\n' "$hits" | sed 's/^/        /' >&2
	fi
done

# --- relative link integrity ---------------------------------------------
# lychee checks the same thing in CI, but only for the files it is pointed at
# and only when the network cooperates. This is the offline half and it is the
# one that catches a rename.
link_failures=0
while IFS=$'\t' read -r source target; do
	# Fragment-only links, external schemes and protocol-relative URLs are
	# somebody else's problem.
	case "$target" in
	'' | '#'* | *://* | mailto:* | tel:* | '//'*) continue ;;
	esac

	target="${target%%#*}"
	target="${target%%\?*}"
	[ -n "$target" ] || continue

	case "$target" in
	/*) resolved="${repo_root}${target}" ;;
	*) resolved="$(dirname -- "$source")/$target" ;;
	esac

	if [ ! -e "$resolved" ]; then
		fail "$source: link target does not exist: $target"
		link_failures=$((link_failures + 1))
	fi
done < <(
	# shellcheck disable=SC2016 # $0 below is awk's record, not a shell parameter
	find . -path ./.git -prune -o -name '*.md' -print0 |
		xargs -0 -r awk '
			FNR == 1 { fenced = 0 }
			# Fenced blocks hold Go and SQL that trips the link pattern.
			/^[[:space:]]*(```|~~~)/ { fenced = !fenced; next }
			fenced { next }
			{
				line = $0
				gsub(/`[^`]*`/, "", line)
				while (match(line, /\]\([^)]*\)/)) {
					target = substr(line, RSTART + 2, RLENGTH - 3)
					line = substr(line, RSTART + RLENGTH)
					# "](path \"title\")" - the title is not part of the target.
					sub(/[[:space:]].*$/, "", target)
					gsub(/^</, "", target)
					gsub(/>$/, "", target)
					print FILENAME "\t" target
				}
			}
		'
)
[ "$link_failures" -eq 0 ] && ok "every relative markdown link resolves on disk"

if [ "$failures" -ne 0 ]; then
	printf '\n%d structural assertion(s) failed\n' "$failures" >&2
	exit 1
fi

printf '\nspec corpus is structurally sound\n'
