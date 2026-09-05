package shell

// PatientOption is one entry the switcher offers: name AND date of birth
// (FR-014), so twins or same-named relatives are distinguishable.
type PatientOption struct {
	ID        string
	Name      string
	BirthDate string
	PhotoURL  string
	Active    bool
}

// PatientSwitcherProps is contracts/pages.md's shell control: every patient
// the account holder owns, and the address the selection is PUT to.
type PatientSwitcherProps struct {
	Options []PatientOption
	Href    string
}

// Label renders one option's accessible text: name and date of birth
// together, never one without the other.
func (o PatientOption) Label() string {
	if o.BirthDate == "" {
		return o.Name
	}

	return o.Name + " (" + o.BirthDate + ")"
}

// Active is the person currently in view, if any — FR-014's other half: the
// switcher shows who that is by name and photograph, not only by a selected
// <option> a screen reader still has to open the list to hear.
func (p PatientSwitcherProps) Active() (PatientOption, bool) {
	for _, option := range p.Options {
		if option.Active {
			return option, true
		}
	}

	return PatientOption{}, false
}
