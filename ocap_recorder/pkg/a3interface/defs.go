package a3interface

/*
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
*/
import "C"

// ConfigStruct is the central configuration used by this library
type configStruct struct {

	// rvExtensionVersion is the value that will be returned when the extension is first called by Arma. This is a string value and is logged by the game engine to the RPT file
	rvExtensionVersion string

	// rvExtensionFunc is the callback function that will be run when an SQF command is sent in the form "x" callExtension "command". The return values should consist of a string to send back to Arma and an error value. If the error value is not nil, the error will be sent to the main program via the ErrChan channel and returned to Arma as the response
	rvExtensionFunc func(command string) (response string, err error)

	// rvExtensionArgsFunc is the callback function that will be run when an SQF command is sent in the form "x" callExtension ["command", ["data"]]. The return values should consist of a string to send back to Arma and an error value. If the error value is not nil, the error will be sent to the main program via the ErrChan channel and returned to Arma as the response
	rvExtensionArgsFunc func(
		command string, data []string,
	) (
		response string, err error,
	)

	// errFunc is the callback function that will be run when an error occurs in the extension. If this isn't defined, errors will still be sent back to Arma as the response to callExtension.
	errFunc func(command string, err error)
}

// SetVersion sets the version string that will be returned when the extension is first called by Arma. This is a string value and is logged by the game engine to the RPT file
func SetVersion(version string) {
	Config.rvExtensionVersion = version
}

// OnCallExtension sets the callback function that will be run when an SQF command is sent in the form "x" callExtension "command"
func OnCallExtension(callback func(command string) (response string, err error)) {
	Config.rvExtensionFunc = callback
}

// OnCallExtensionArgs sets the callback function that will be run when an SQF command is sent in the form "x" callExtension ["command", ["data"]]
func OnCallExtensionArgs(
	callback func(command string, data []string) (response string, err error),
) {
	Config.rvExtensionArgsFunc = callback
}

// OnError sets the callback function that will be run when an error occurs in the extension. This defaults to the native logger. Even if this isn't defined, errors will still be sent back to Arma as the response to callExtension.
func OnError(
	callback func(command string, err error),
) {
	Config.errFunc = callback
}
