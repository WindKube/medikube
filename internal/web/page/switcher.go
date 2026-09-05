package page

import (
	"context"

	"medikube/internal/domain/access"
	"medikube/internal/service/patient"
	"medikube/internal/web/api"
	"medikube/internal/web/views/shell"
)

// switcherLimit is generous rather than exact: contracts/pages.md's seeded
// account holds 25 patients (SC-002's fixture), and a switcher that silently
// dropped somebody past a small page size would be worse than one large
// enough that it never has to.
const switcherLimit = 100

// patientSwitcherProps resolves FR-014's shell control for one request: every
// patient the actor owns, and which of them (if any) is the person in view.
func patientSwitcherProps(ctx context.Context, actor access.Actor, resolve api.PatientResolve) (shell.PatientSwitcherProps, error) {
	svc, err := resolve()
	if err != nil {
		return shell.PatientSwitcherProps{}, err
	}

	listing, err := svc.List(ctx, actor, patient.Query{Limit: switcherLimit})
	if err != nil {
		return shell.PatientSwitcherProps{}, err
	}

	active, err := svc.ResolveActivePatient(ctx, actor)
	if err != nil {
		return shell.PatientSwitcherProps{}, err
	}

	href, err := switcherHref()
	if err != nil {
		return shell.PatientSwitcherProps{}, err
	}

	var activeID string
	if active != nil {
		activeID = active.ID
	}

	options := make([]shell.PatientOption, 0, len(listing.Items))
	for _, p := range listing.Items {
		var photoURL string
		if p.HasPhoto {
			photoURL = api.PatientPhotoURL(p.ID)
		}

		options = append(options, shell.PatientOption{
			ID:        p.ID,
			Name:      p.FirstName + " " + p.LastName,
			BirthDate: p.BirthDate.String(),
			PhotoURL:  photoURL,
			Active:    p.ID == activeID,
		})
	}

	return shell.PatientSwitcherProps{Options: options, Href: href}, nil
}

// switcherHref is the setActivePatient route, resolved from the table rather
// than composed, mirroring every other address this package hands a page.
func switcherHref() (string, error) {
	paths, err := routePaths(map[string]string{api.OpSetActivePatient: ""})
	if err != nil {
		return "", err
	}

	return paths[api.OpSetActivePatient], nil
}
