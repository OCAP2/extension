package a3interface

/*
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
*/
import "C"
import (
	"fmt"
	"log"
	"unsafe"
)

// Config defines how calls to this extension will be handled
// it can be configured using method calls against it
var Config configStruct = configStruct{
	rvExtensionVersion: "No version set",
	errFunc: func(command string, err error) {
		log.Printf("Error with an extension call. Name: %s, Error: %s\n", command, err.Error())
	},
}

// RVExtensionVersion is called by Arma to get the version of the extension
//
//export RVExtensionVersion
func RVExtensionVersion(output *C.char, outputsize C.size_t) {
	result := Config.rvExtensionVersion
	replyToSyncArmaCall(result, output, outputsize)
}

// RVExtension is called by Arma when in the format of: "extensionName" callExtension "command"
//
//export RVExtension
func RVExtension(output *C.char, outputsize C.size_t, input *C.char) {

	var command string = C.GoString(input)
	var response string = fmt.Sprintf(`No responses configured!`)

	if Config.rvExtensionFunc == nil {
		replyToSyncArmaCall(response, output, outputsize)
		return
	}

	response, err := Config.rvExtensionFunc(command)
	if err != nil {
		if Config.errFunc != nil {
			Config.errFunc(command, err)
		}
		replyToSyncArmaCall(err.Error(), output, outputsize)
		return
	}
	replyToSyncArmaCall(response, output, outputsize)
}

// RVExtensionArgs is called by Arma when in the format of: "extensionName" callExtension ["command", ["data"]]
//
//export RVExtensionArgs
func RVExtensionArgs(output *C.char, outputsize C.size_t, input *C.char, argv **C.char, argc C.int) {

	var err error
	// get command as Go string
	command := C.GoString(input)
	// set default response
	response := fmt.Sprintf(`["Function: %s", "nb params: %d"]`, command, argc)
	// check if the callback function is set and return default value if not
	if Config.rvExtensionArgsFunc == nil {
		replyToSyncArmaCall(response, output, outputsize)
		return
	}

	// process the C vector into a Go slice
	var offset = unsafe.Sizeof(uintptr(0))
	var data []string
	for index := C.int(0); index < argc; index++ {
		data = append(data, C.GoString(*argv))
		argv = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(argv)) + offset))
	}

	// run the callback function
	response, err = Config.rvExtensionArgsFunc(command, data)
	if err != nil {
		if Config.errFunc != nil {
			Config.errFunc(command, err)
		}
		replyToSyncArmaCall(err.Error(), output, outputsize)
		return
	}
	replyToSyncArmaCall(response, output, outputsize)
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
