package api

import (
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
)

// units.go is US3-6's presentation-edge unit conversion (research D-15):
// every vitals value is stored in SI and this file is the only place that
// converts it to and from the actor's own unit_system. Nothing in
// internal/service/vitals or internal/store/vitals converts anything.

func weightToDisplay(kg *float64, system identity.UnitSystem) *float64 {
	return convertMeasurement(kg, system, clinical.KgToLb)
}

func weightToSI(value *float64, system identity.UnitSystem) *float64 {
	return convertMeasurement(value, system, clinical.LbToKg)
}

func heightToDisplay(cm *float64, system identity.UnitSystem) *float64 {
	return convertMeasurement(cm, system, clinical.CmToIn)
}

func heightToSI(value *float64, system identity.UnitSystem) *float64 {
	return convertMeasurement(value, system, clinical.InToCm)
}

func temperatureToDisplay(celsius *float64, system identity.UnitSystem) *float64 {
	return convertMeasurement(celsius, system, clinical.CelsiusToFahrenheit)
}

func temperatureToSI(value *float64, system identity.UnitSystem) *float64 {
	return convertMeasurement(value, system, clinical.FahrenheitToCelsius)
}

func glucoseToDisplay(mmolL *float64, system identity.UnitSystem) *float64 {
	return convertMeasurement(mmolL, system, clinical.MmolLToMgDl)
}

func glucoseToSI(value *float64, system identity.UnitSystem) *float64 {
	return convertMeasurement(value, system, clinical.MgDlToMmolL)
}

// convertMeasurement applies convert only for an imperial-preferring actor,
// and leaves a metric one's value untouched — the identity conversion for
// everybody who has not asked for one.
func convertMeasurement(value *float64, system identity.UnitSystem, convert func(float64) float64) *float64 {
	if value == nil {
		return nil
	}

	if system != identity.UnitSystemImperial {
		v := *value

		return &v
	}

	v := convert(*value)

	return &v
}
