// Package config loads the process configuration from the environment into one
// validated struct at boot.
//
// It is the only configuration mechanism MediKube defines: there are no
// configuration files, and a rejected environment reports every problem at
// once rather than one per restart.
package config
