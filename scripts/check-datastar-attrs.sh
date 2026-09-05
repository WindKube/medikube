#!/usr/bin/env bash
# T004. MediKube ships the free Datastar bundle only (research/frontend.md
# Trap 5, SHARED-DESIGN.md §"Only the free Datastar attribute set may be
# used"). Nine attributes and two actions on the "Reference / Attributes" docs
# page are Pro and require a commercial licence MediKube does not have; used
# anyway, they silently do nothing at runtime. This scans every .templ source
# — never the generated *_templ.go, which is markup text by the time it is
# Go source and would only echo what the source already says — for a Pro
# attribute, and separately for the deprecated v0 `data-on-<event>` hyphen
# delimiter (v1's free form is `data-on:<event>`, colon-delimited).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
targets="$root/internal/web/views"

pro_attrs=(
  "data-persist" "data-query-string" "data-replace-url" "data-scroll-into-view"
  "data-view-transition" "data-custom-validity" "data-animate" "data-match-media"
  "data-on-raf" "data-on-resize" "@clipboard" "@fit"
)

found=0

if [ -d "$targets" ]; then
  files=$(find "$targets" -name '*.templ')

  for attr in "${pro_attrs[@]}"; do
    hits=$(grep -rnF -- "$attr" $files 2>/dev/null || true)
    if [ -n "$hits" ]; then
      echo "forbidden Datastar Pro attribute $attr:"
      echo "$hits"
      found=1
    fi
  done

  # The v0 hyphen delimiter, any DOM event name: data-on-click, data-on-submit,
  # and so on. data-on-raf and data-on-resize are already caught above as Pro
  # attributes; data-on-intersect, data-on-interval and data-on-signal-patch are
  # free-bundle attributes whose own names are hyphenated (they are not an
  # event binding at all) and are excluded here rather than flagged.
  hits=$(grep -rnE -- 'data-on-[a-z]' $files 2>/dev/null \
    | grep -vE 'data-on-(raf|resize|intersect|interval|signal-patch)' || true)
  if [ -n "$hits" ]; then
    echo "forbidden Datastar v0 delimiter (use data-on:<event>, not data-on-<event>):"
    echo "$hits"
    found=1
  fi
fi

if [ "$found" -ne 0 ]; then
  echo "task lint:datastar: one or more forbidden Datastar attributes found above" >&2
  exit 1
fi

echo "task lint:datastar: clean"
