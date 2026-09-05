#!/usr/bin/env bash
# Generates specs/002-patient-core/traceability.md from spec.md and tasks.md.
#
# Three tables:
#   1. Every acceptance scenario -> its named test. Scenario text is read out
#      of spec.md; the test is not derivable from prose, so it is the one
#      hand-maintained table below. Kept honest by a check that the named
#      file actually declares the named test.
#   2. Every FR-001..FR-056 -> the task ids that mention it in tasks.md.
#   3. Every SC-001..SC-014 -> the task ids that mention it, or the
#      "[outcome metric]" spec.md marks it with when no task does.
#
# Fails (non-zero exit, nothing written) if any row would be empty.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
spec="$root/specs/002-patient-core/spec.md"
tasks="$root/specs/002-patient-core/tasks.md"
out="$root/specs/002-patient-core/traceability.md"

# --- 1. Scenario -> test. Hand-maintained: prose does not name a test. -----
declare -a scenario_ids=()
declare -A scenario_test=()

add() { scenario_ids+=("$1"); scenario_test["$1"]="$2"; }

add US1-1 "internal/web/api/register_selfrecord_test.go::TestRegisteringProvisionsExactlyOneSelfRecordPatient"
add US1-2 "internal/service/patient/service_test.go::TestCreateSavesAndOwnsThePatient"
add US1-3 "internal/service/patient/service_test.go::TestCreateReportsEveryInvalidFieldAtOnce"
add US1-4 "internal/service/patient/service_test.go::TestASecondSelfRecordIsRefusedWithConflict"
add US1-5 "internal/store/patient/photo_integration_test.go::TestReplacingRemovesTheOldOriginalAndBothOldThumbnails"
add US1-6 "internal/web/views/patients/detail_test.go::TestAbsentDetailsDoNotRenderAsZeroOrBlank"
add US1-7 "internal/service/patient/service_test.go::TestUpdateRefusesAStaleVersion"
add US1-8 "internal/web/api/patients_authz_test.go::TestAnAnonymousRequestForAPatientOrPhotoDisclosesNothing"

add US2-1 "internal/store/migrations/repoint_test.go::TestRepointBackfillsEveryMedicationToItsOwnersSelfRecord"
add US2-2 "internal/web/api/medications_scope_test.go::TestAListForOnePatientHoldsOnlyThatPatientsMedications"
add US2-3 "internal/service/medication/service_test.go::TestCreateRefusesAnEmptyPatient"
add US2-4 "internal/web/api/medications_scope_test.go::TestAPatchNamingAPatientIsRefusedAndTheRecordIsUnchanged"
add US2-5 "internal/web/api/medications_scope_test.go::TestAListNamingAnotherAccountsPatientIsAMiss"
add US2-6 "internal/web/page/medications_test.go::TestTheListPageNamesThePatientItShows"

add US3-1 "e2e/patient-switch.spec.ts::names the person in view across three screens, and the choice survives a reload"
add US3-2 "internal/web/api/active_patient_test.go::TestTheActivePatientPointerSurvivesSignOutAndSignIn"
add US3-3 "internal/store/patient/active_unset_integration_test.go::TestDeletingAPatientNullsTheActivePatientPointerEverywhereItPointed"
add US3-4 "internal/service/patient/active_test.go::TestResolveActivePatientAutoSelectsExactlyOne"
add US3-5 "internal/web/api/active_patient_test.go::TestSwitchingPatientsGrantsNoAccessToAnotherAccountsRecords"
add US3-6 "internal/web/views/records/medication_form_test.go::TestEveryRecordedFieldHasAControl"

add US4-1 "internal/web/api/patient_chart_test.go::TestGetPatientChartCountsOnlyThisPatientsMedications"
add US4-2 "internal/web/views/patients/detail_test.go::TestTheDetailLandmarkHoldsBothThePopulatedAndTheEmptyChart"
add US4-3 "internal/web/api/patient_chart_units_test.go::TestGetPatientChartUnitSystemChangesOnlyTheDisplayBlock"
add US4-4 "internal/domain/person/age_test.go::TestAgeAt"
add US4-5 "internal/web/api/patient_chart_test.go::TestGetPatientChartActivityEntryCarriesNoContentAndReportsADeletedTarget"
add US4-6 "internal/store/patient/chart_bench_test.go::BenchmarkPatientChartSummaryWith50kMedications"

add US5-1 "internal/web/api/practitioners_test.go::TestCreatePractitioner"
add US5-2 "internal/web/api/facilities_test.go::TestListFacilities"
add US5-3 "internal/service/facility/service_test.go::TestCreateAcceptsTwoFacilitiesSharingAName"
add US5-4 "internal/service/practitioner/service_test.go::TestCreateRefusesADuplicateNameAndSpecialty"
add US5-5 "internal/store/practitioner/delete_unset_integration_test.go::TestDeletingAPractitionerClearsEveryReference"
add US5-6 "internal/web/api/practitioners_test.go::TestListPractitioners"

add US6-1 "internal/web/views/patients/delete_confirm_test.go::TestDeleteConfirmNamesThePersonAndStatesTheRecordCount"
add US6-2 "internal/store/patient/delete_integration_test.go::TestDeletingAPatientDestroysItsMedicationsAndItsPhoto"
add US6-3 "internal/store/patient/delete_integration_test.go::TestDeletingAPatientLeavesNoRowAnywhereReferencingIt"
add US6-4 "internal/web/api/patients_delete_test.go::TestDeletePatientRefusesTheSelfRecordWith409"
add US6-5 "internal/web/api/patients_delete_test.go::TestDeletePatientWritesExactlyOneAuditRowCarryingNoName"
add US6-6 "internal/web/api/patients_delete_test.go::TestDeletePatientRefusesAStrangerAsNotFoundAndAudits"

# Scenario text, derived from spec.md rather than retyped.
scenario_text_file="$(mktemp)"
fr_rows_file="$(mktemp)"
sc_rows_file="$(mktemp)"
trap 'rm -f "$scenario_text_file" "$fr_rows_file" "$sc_rows_file"' EXIT
awk '
BEGIN{story=0; capture=0; started=0}
/^### User Story/{story++; next}
/^\*\*Acceptance Scenarios\*\*:/{capture=1; started=0; next}
capture && /^[0-9]+\. /{
  started=1
  match($0, /^[0-9]+/); n=substr($0,RSTART,RLENGTH);
  text=$0; sub(/^[0-9]+\. /,"",text);
  printf "US%d-%s\t%s\n", story, n, text
  next
}
capture && started && /^$/{capture=0}
' "$spec" > "$scenario_text_file"

declare -A scenario_text=()
while IFS=$'\t' read -r id text; do
  scenario_text["$id"]="$text"
done < "$scenario_text_file"

if [ "${#scenario_text[@]}" -ne 38 ]; then
  echo "gen-traceability: spec.md yielded ${#scenario_text[@]} acceptance scenarios, expected 38" >&2
  exit 1
fi

for id in "${scenario_ids[@]}"; do
  if [ -z "${scenario_text[$id]:-}" ]; then
    echo "gen-traceability: $id has no scenario text in spec.md (renumbered?)" >&2
    exit 1
  fi
  file="${scenario_test[$id]%%::*}"
  test_name="${scenario_test[$id]#*::}"
  if [ ! -f "$root/$file" ]; then
    echo "gen-traceability: $id points at $file, which does not exist" >&2
    exit 1
  fi
  case "$file" in
    *.go)
      if ! grep -qF "func $test_name(" "$root/$file"; then
        echo "gen-traceability: $file no longer declares $test_name (row for $id)" >&2
        exit 1
      fi
      ;;
    *.spec.ts)
      if ! grep -qF "$test_name" "$root/$file"; then
        echo "gen-traceability: $file no longer contains the case named for $id" >&2
        exit 1
      fi
      ;;
  esac
done

# --- 2. FR-001..FR-056 -> task ids. -----------------------------------------
#
# Most requirements are named directly on a task line (grep for that). A
# requirement named only in a phase's own "**Covers**: FR-x … FR-y" summary
# line — never repeated per task — falls back to every task id in that
# phase's block. What is left after both (FR-054..FR-056, this phase's own
# cross-cutting requirements) falls back to plan.md's numbered Phase Exit
# Criteria.
plan="$root/specs/002-patient-core/plan.md"

phase_blocks_file="$(mktemp)"
exit_criteria_file="$(mktemp)"
trap 'rm -f "$scenario_text_file" "$fr_rows_file" "$sc_rows_file" "$phase_blocks_file" "$exit_criteria_file"' EXIT

awk '
BEGIN{block=""; covers=""; tasks=""}
/^## Phase/{
  if (block != "") { print covers "\t" tasks }
  block=$0; covers=""; tasks=""; next
}
/^\*\*Covers\*\*:/{
  line=$0
  sub(/^\*\*Covers\*\*: */,"",line)
  covers=line
  next
}
/^- \[[ x]\] T[0-9]+[a-z]?/{
  match($0, /T[0-9]+[a-z]?/)
  tasks = tasks " " substr($0, RSTART, RLENGTH)
}
END{ if (block != "") print covers "\t" tasks }
' "$tasks" > "$phase_blocks_file"

declare -A phase_fallback=()
while IFS=$'\t' read -r covers block_tasks; do
  [ -z "$covers" ] && continue
  frpart="${covers%%. Acceptance*}"
  block_ids=$(echo "$block_tasks" | xargs -n1 | sort -u -V | paste -sd, -)
  IFS=',' read -ra ranges <<< "$frpart"
  for r in "${ranges[@]}"; do
    r_trim=$(echo "$r" | xargs)
    if [[ "$r_trim" == *"…"* ]]; then
      lo=$(echo "$r_trim" | sed -E 's/FR-([0-9]+).*/\1/')
      hi=$(echo "$r_trim" | sed -E 's/.*FR-([0-9]+)$/\1/')
      for n in $(seq "$((10#$lo))" "$((10#$hi))"); do
        phase_fallback["$(printf 'FR-%03d' "$n")"]="$block_ids"
      done
    elif [[ "$r_trim" == FR-* ]]; then
      phase_fallback["$r_trim"]="$block_ids"
    fi
  done
done < "$phase_blocks_file"

awk '
BEGIN{flag=0; n=""; item=""}
/^## Phase Exit Criteria/{flag=1; next}
flag && /^[0-9]+\. /{
  if (n != "") print n "\t" item
  match($0,/^[0-9]+/); n=substr($0,RSTART,RLENGTH); item=$0
  next
}
flag && /^---/{ if (n != "") print n "\t" item; flag=0; next }
flag && n != "" { item = item " " $0 }
END{ if (flag && n != "") print n "\t" item }
' "$plan" > "$exit_criteria_file"

exit_criterion_for() {
  local id="$1"
  while IFS=$'\t' read -r n line; do
    if [[ "$line" == *"$id"* ]]; then
      echo "Phase Exit Criterion $n"
      return 0
    fi
  done < "$exit_criteria_file"
  return 1
}

fr_count=$(grep -oE 'FR-[0-9]{3}' "$spec" | sort -u | wc -l | tr -d ' ')
if [ "$fr_count" -eq 0 ]; then
  echo "gen-traceability: spec.md declares no FR-nnn requirements" >&2
  exit 1
fi

# shellcheck disable=SC2013 # each match is one FR-nnn word, never a line with spaces
for fr in $(grep -oE 'FR-[0-9]{3}' "$spec" | sort -u); do
  ids=$( { grep -E "\\b${fr}\\b" "$tasks" \
    | grep -oE '^- \[[ x]\] T[0-9]+[a-z]?' \
    | grep -oE 'T[0-9]+[a-z]?' \
    | sort -u -V \
    | paste -sd, - ; } || true)
  if [ -z "$ids" ]; then
    ids="${phase_fallback[$fr]:-}"
  fi
  if [ -z "$ids" ]; then
    ids="$(exit_criterion_for "$fr" || true)"
  fi
  if [ -z "$ids" ]; then
    echo "gen-traceability: $fr is not named on any task line, in any phase's Covers line, or in a Phase Exit Criterion" >&2
    exit 1
  fi
  echo "| $fr | $ids |" >> "$fr_rows_file"
done

# --- 3. SC-001..SC-014 -> task ids, a Phase Exit Criterion, or an outcome
#        metric. --------------------------------------------------------------
# shellcheck disable=SC2013 # each match is one SC-nnn word, never a line with spaces
for sc in $(grep -oE 'SC-[0-9]{3}' "$spec" | sort -u); do
  ids=$( { grep -E "\\b${sc}\\b" "$tasks" \
    | grep -oE '^- \[[ x]\] T[0-9]+[a-z]?' \
    | grep -oE 'T[0-9]+[a-z]?' \
    | sort -u -V \
    | paste -sd, - ; } || true)
  if [ -n "$ids" ]; then
    echo "| $sc | $ids |" >> "$sc_rows_file"
    continue
  fi
  if grep -E "\\*\\*${sc}\\*\\* \\*\\[outcome metric\\]\\*" "$spec" > /dev/null; then
    echo "| $sc | [outcome metric] (spec.md) |" >> "$sc_rows_file"
    continue
  fi
  criterion="$(exit_criterion_for "$sc" || true)"
  if [ -n "$criterion" ]; then
    echo "| $sc | $criterion |" >> "$sc_rows_file"
    continue
  fi
  echo "gen-traceability: $sc is on no task line, marked no [outcome metric] in spec.md, and named in no Phase Exit Criterion" >&2
  exit 1
done

# --- Assemble ---------------------------------------------------------------
{
  echo "# Traceability: Patient Core"
  echo
  echo "Generated by \`scripts/gen-traceability.sh\`. Do not hand-edit — the scenario→test"
  echo "table lives in the script (prose does not name a test); the FR and SC tables are"
  echo "derived from \`spec.md\` and \`tasks.md\` on every run. Regenerate after any FR/SC/task"
  echo "renumbering: \`scripts/gen-traceability.sh > specs/002-patient-core/traceability.md\`."
  echo
  echo "## Acceptance scenarios (spec.md) -> named test"
  echo
  echo "| Scenario | Test |"
  echo "|---|---|"
  for id in "${scenario_ids[@]}"; do
    # shellcheck disable=SC2016 # printf format string; %s is meant literal here
    printf '| %s: %s | `%s` |\n' "$id" "${scenario_text[$id]}" "${scenario_test[$id]}"
  done
  echo
  echo "## Functional requirements (FR-001..FR-$(printf '%03d' "$fr_count")) -> tasks"
  echo
  echo "| Requirement | Tasks |"
  echo "|---|---|"
  cat "$fr_rows_file"
  echo
  echo "## Success criteria -> tasks, or an outcome metric"
  echo
  echo "| Criterion | Tasks / marker |"
  echo "|---|---|"
  cat "$sc_rows_file"
} > "$out"

echo "gen-traceability: wrote $out" >&2
