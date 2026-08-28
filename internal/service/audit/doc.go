// Package audit records what was done to patient data and purges the trail when
// its retention window expires.
//
// Its writer takes an actor, an action, a target identifier and a time, and has
// no parameter through which content could be recorded.
package audit
