package a3interface

/*
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
*/
import "C"

import (
	"github.com/OCAP2/extension/internal/dispatcher"
)

// ConfigStruct is the central configuration used by this library
type configStruct struct {
	// rvExtensionVersion is the value that will be returned when the extension is first called by Arma
	rvExtensionVersion string

	// dispatcher handles event routing
	dispatcher *dispatcher.Dispatcher
}

// Init method initializes the config struct
func (c *configStruct) Init() {
	c.rvExtensionVersion = "No version set"
}

// SetVersion sets the version string that will be returned when the extension is first called by Arma
func SetVersion(version string) {
	Config.rvExtensionVersion = version
}

// SetDispatcher sets the event dispatcher for handling commands
func SetDispatcher(d *dispatcher.Dispatcher) {
	Config.dispatcher = d
}

// GetDispatcher returns the configured dispatcher, or nil if not set
func GetDispatcher() *dispatcher.Dispatcher {
	return Config.dispatcher
}
