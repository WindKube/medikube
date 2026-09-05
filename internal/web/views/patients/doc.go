// Package patients renders contracts/pages.md P1 and P2: the patient list and
// the patient chart's own header, plus the create/edit form and the photo
// control both embed.
//
// Patients are not a kind.Kind (research D-05, internal/web/cursor.go), so
// this package is bespoke rather than a Views implementation of
// internal/records — there is no generic list/detail/form machinery to plug
// into here, the way internal/web/views/records does for medications.
package patients
