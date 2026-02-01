package a3interface

/*
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"github.com/OCAP2/extension/v5/internal/dispatcher"
)

// Config defines how calls to this extension will be handled
var Config configStruct = configStruct{}

func init() {
	Config.Init()
}

// called by Arma to get the version of the extension
//
//export RVExtensionVersion
func RVExtensionVersion(output *C.char, outputsize C.size_t) {
	defer recoverPanic("RVExtensionVersion", output, outputsize)
	result := Config.rvExtensionVersion
	replyToSyncArmaCall(result, output, outputsize)
}

// called by Arma when in the format of: "extensionName" callExtension "command"
//
//export RVExtension
func RVExtension(output *C.char, outputsize C.size_t, input *C.char) {
	defer recoverPanic("RVExtension", output, outputsize)
	command := C.GoString(input)
	commandSubstr := strings.Split(command, "|")[0]

	// Handle built-in timestamp command
	if command == ":TIMESTAMP:" {
		replyToSyncArmaCall(getTimestamp(), output, outputsize)
		return
	}

	// Use dispatcher (check both full command and substring)
	if Config.dispatcher != nil {
		dispatchCommand := command
		if !Config.dispatcher.HasHandler(command) && Config.dispatcher.HasHandler(commandSubstr) {
			dispatchCommand = commandSubstr
		}

		if Config.dispatcher.HasHandler(dispatchCommand) {
			event := dispatcher.Event{
				Command:   dispatchCommand,
				Args:      []string{command}, // pass full command as arg for legacy compat
				Timestamp: time.Now(),
			}

			result, err := Config.dispatcher.Dispatch(event)
			response := formatDispatchResponse(dispatchCommand, result, err)
			replyToSyncArmaCall(response, output, outputsize)
			return
		}
	}

	// No handler found
	replyToSyncArmaCall(fmt.Sprintf(`["error", "%s", "no handler registered"]`, command), output, outputsize)
}

// called by Arma when in the format of: "extensionName" callExtension ["command", ["data"]]
//
//export RVExtensionArgs
func RVExtensionArgs(output *C.char, outputsize C.size_t, input *C.char, argv **C.char, argc C.int) {
	defer recoverPanic("RVExtensionArgs", output, outputsize)
	command := C.GoString(input)
	args := parseArgsFromC(argv, argc)

	// Use dispatcher
	if Config.dispatcher != nil && Config.dispatcher.HasHandler(command) {
		event := dispatcher.Event{
			Command:   command,
			Args:      args,
			Timestamp: time.Now(),
		}

		result, err := Config.dispatcher.Dispatch(event)
		response := formatDispatchResponse(command, result, err)
		replyToSyncArmaCall(response, output, outputsize)
		return
	}

	// No handler found
	replyToSyncArmaCall(fmt.Sprintf(`["error", "%s", "no handler registered"]`, command), output, outputsize)
}

// parseArgsFromC converts C argv array to Go string slice
func parseArgsFromC(argv **C.char, argc C.int) []string {
	var offset = unsafe.Sizeof(uintptr(0))
	var data []string
	for index := C.int(0); index < argc; index++ {
		data = append(data, C.GoString(*argv))
		argv = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(argv)) + offset))
	}
	return data
}

// formatDispatchResponse formats the dispatcher result for ArmA.
// Format: ["ok", result] for success, ["error", "message"] for errors.
// Uses JSON encoding for proper SQF-compatible array/object formatting.
func formatDispatchResponse(command string, result any, err error) string {
	if err != nil {
		return fmt.Sprintf(`["error", "%s"]`, err.Error())
	}
	if result == nil {
		return `["ok"]`
	}
	switch v := result.(type) {
	case string:
		return fmt.Sprintf(`["ok", "%s"]`, v)
	default:
		jsonBytes, jsonErr := json.Marshal(result)
		if jsonErr != nil {
			return fmt.Sprintf(`["error", "failed to encode: %s"]`, jsonErr.Error())
		}
		return fmt.Sprintf(`["ok", %s]`, string(jsonBytes))
	}
}

// recoverPanic catches any panic in the exported functions and returns a safe error response.
// This prevents the game from crashing due to unhandled Go panics.
func recoverPanic(funcName string, output *C.char, outputsize C.size_t) {
	if r := recover(); r != nil {
		errMsg := fmt.Sprintf(`["error", "%s", "panic recovered: %v"]`, funcName, r)
		replyToSyncArmaCall(errMsg, output, outputsize)
	}
}

// replyToSyncArmaCall will respond to a synchronous extension call from Arma
func replyToSyncArmaCall(response string, output *C.char, outputsize C.size_t) {
	result := C.CString(response)
	defer C.free(unsafe.Pointer(result))
	var size = C.strlen(result) + 1
	if size > outputsize {
		size = outputsize
	}
	C.memmove(unsafe.Pointer(output), unsafe.Pointer(result), size)
}

func getTimestamp() string {
	return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
}
