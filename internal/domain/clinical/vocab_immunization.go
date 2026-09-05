package clinical

import "slices"

// ImmunizationRoute is how a vaccine was given.
type ImmunizationRoute string

const (
	ImmunizationRouteIntramuscular ImmunizationRoute = "intramuscular"
	ImmunizationRouteSubcutaneous  ImmunizationRoute = "subcutaneous"
	ImmunizationRouteIntradermal   ImmunizationRoute = "intradermal"
	ImmunizationRouteOral          ImmunizationRoute = "oral"
	ImmunizationRouteIntranasal    ImmunizationRoute = "intranasal"
)

// ImmunizationSite is where on the body, with a catch-all.
type ImmunizationSite string

const (
	ImmunizationSiteLeftArm    ImmunizationSite = "left_arm"
	ImmunizationSiteRightArm   ImmunizationSite = "right_arm"
	ImmunizationSiteLeftThigh  ImmunizationSite = "left_thigh"
	ImmunizationSiteRightThigh ImmunizationSite = "right_thigh"
	ImmunizationSiteOral       ImmunizationSite = "oral"
	ImmunizationSiteNasal      ImmunizationSite = "nasal"
	ImmunizationSiteOther      ImmunizationSite = "other"
)

var (
	immunizationRoutes = []ImmunizationRoute{
		ImmunizationRouteIntramuscular, ImmunizationRouteSubcutaneous,
		ImmunizationRouteIntradermal, ImmunizationRouteOral, ImmunizationRouteIntranasal,
	}

	immunizationSites = []ImmunizationSite{
		ImmunizationSiteLeftArm, ImmunizationSiteRightArm, ImmunizationSiteLeftThigh,
		ImmunizationSiteRightThigh, ImmunizationSiteOral, ImmunizationSiteNasal, ImmunizationSiteOther,
	}
)

func ImmunizationRoutes() []ImmunizationRoute { return slices.Clone(immunizationRoutes) }
func ImmunizationSites() []ImmunizationSite   { return slices.Clone(immunizationSites) }

func (v ImmunizationRoute) Valid() bool { return slices.Contains(immunizationRoutes, v) }
func (v ImmunizationSite) Valid() bool  { return slices.Contains(immunizationSites, v) }
