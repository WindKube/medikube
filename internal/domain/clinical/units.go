package clinical

// units.go converts between MediKube's SI storage and the account holder's
// preferred unit system (research D-15). Conversion is pure and has no notion
// of a user or a request: the caller supplies the unit system, which is
// internal/web's job at the presentation edge, never the service's, the
// repository's or the database's.

const (
	kgPerLb     = 0.45359237
	cmPerIn     = 2.54
	mmolPerMgdl = 0.0555 // 1 mg/dL glucose ≈ 0.0555 mmol/L
)

func KgToLb(kg float64) float64 { return kg / kgPerLb }
func LbToKg(lb float64) float64 { return lb * kgPerLb }

func CmToIn(cm float64) float64 { return cm / cmPerIn }
func InToCm(in float64) float64 { return in * cmPerIn }

func CelsiusToFahrenheit(c float64) float64 { return c*9/5 + 32 }
func FahrenheitToCelsius(f float64) float64 { return (f - 32) * 5 / 9 }

func MmolLToMgDl(mmol float64) float64 { return mmol / mmolPerMgdl }
func MgDlToMmolL(mgdl float64) float64 { return mgdl * mmolPerMgdl }

// BMI is weight_kg / (height_cm/100)^2, MediKube's one formula for it
// (data-model §4.7). Zero height returns zero rather than dividing by it: a
// caller with no height on file has no BMI to derive, not an error to handle.
func BMI(weightKg, heightCm float64) float64 {
	if heightCm <= 0 {
		return 0
	}

	metres := heightCm / 100

	return weightKg / (metres * metres)
}
