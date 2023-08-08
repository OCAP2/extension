package a3interface

/*
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
*/
import "C"
import (
	"fmt"
	"strings"
	"time"
	"unsafe"
)

// Config defines how calls to this extension will be handled
// it can be configured using method calls against it
var Config configStruct = configStruct{
	rvExtensionVersion: "No version set",
}

// called by Arma to get the version of the extension
//
//export RVExtensionVersion
func RVExtensionVersion(output *C.char, outputsize C.size_t) {
	result := Config.rvExtensionVersion
	replyToSyncArmaCall(result, output, outputsize)
}

// called by Arma when in the format of: "extensionName" callExtension "command"
//
//export RVExtension
func RVExtension(output *C.char, outputsize C.size_t, input *C.char) {

	var command string = C.GoString(input)
	var commandSubstr string = strings.Split(command, "|")[0]
	var desiredCommand string
	var response string = fmt.Sprintf(`No responses configured!`)

	// send default reply immediately
	replyToSyncArmaCall(response, output, outputsize)

	// check if the callback channel is set for this command
	// first with the full command
	if _, ok := Config.rvExtensionChannels[command]; !ok {
		// then with the substring
		if _, ok := Config.rvExtensionChannels[commandSubstr]; !ok {
			// log an error if it isn't
			writeErrChan(command, fmt.Errorf("No channel set for command: %s", command))
			return
		}
		desiredCommand = commandSubstr
	} else {
		desiredCommand = command
	}

	// get channel
	channel := Config.rvExtensionChannels[desiredCommand]
	// send full command to channel
	channel <- command
}

// called by Arma when in the format of: "extensionName" callExtension ["command", ["data"]]
//
//export RVExtensionArgs
func RVExtensionArgs(output *C.char, outputsize C.size_t, input *C.char, argv **C.char, argc C.int) {

	// get command as Go string
	command := C.GoString(input)
	// set default response
	response := fmt.Sprintf(`["Function: %s", "nb params: %d"]`, command, argc)
	// reply as soon as possible so Arma doesn't hang
	replyToSyncArmaCall(response, output, outputsize)

	// check if the callback channel is set for this command
	if _, ok := Config.rvExtensionArgsChannels[command]; !ok {
		// log an error if it isn't
		writeErrChan(command, fmt.Errorf("No channel set for command: %s", command))
		return
	}

	// now, we'll process the data
	// process the C vector into a Go slice
	var offset = unsafe.Sizeof(uintptr(0))
	var data []string
	for index := C.int(0); index < argc; index++ {
		data = append(data, C.GoString(*argv))
		argv = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(argv)) + offset))
	}

	// append timestamp in nanoseconds
	data = append(data, fmt.Sprintf("%d", time.Now().UnixNano()))

	// send the data to the channel
	Config.rvExtensionArgsChannels[command] <- data
}

// replyToSyncArmaCall will respond to a synchronous extension call from Arma
// it returns a single string and any wait time will block Arma
func replyToSyncArmaCall(
	response string,
	output *C.char,
	outputsize C.size_t,
) {
	// Reply to a synchronous call from Arma with a string response
	result := C.CString(response)
	defer C.free(unsafe.Pointer(result))
	var size = C.strlen(result) + 1
	if size > outputsize {
		size = outputsize
	}
	C.memmove(unsafe.Pointer(output), unsafe.Pointer(result), size)
}

// writeErrChan will write an error to the error channel for a command
func writeErrChan(command string, err error) {
	if Config.errChan == nil {
		return
	}
	Config.errChan <- []string{command, err.Error()}
}
