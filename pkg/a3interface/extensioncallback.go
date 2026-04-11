package a3interface

/*
#include <stdlib.h>
#include <stdio.h>
#include <string.h>

typedef int (*extensionCallback)(char const *name, char const *function, char const *data);

static inline int runExtensionCallback(extensionCallback fnc, char const *name, char const *function, char const *data)
{
	return fnc(name, function, data);
}
*/
import "C"
import (
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

var extensionCallbackFnc C.extensionCallback

// callbackSink is an optional hook used by tests. When non-nil,
// WriteArmaCallback diverts its output here instead of invoking the
// real C callback function. Protected by callbackSinkMu.
var (
	callbackSinkMu sync.Mutex
	callbackSink   func(name, function string, data ...string)
)

// SetCallbackSinkForTest installs a hook that receives WriteArmaCallback
// invocations instead of forwarding them to the registered C callback.
// Pass nil to restore the C callback path. Intended for tests only.
func SetCallbackSinkForTest(sink func(name, function string, data ...string)) {
	callbackSinkMu.Lock()
	defer callbackSinkMu.Unlock()
	callbackSink = sink
}

// RVExtensionRegisterCallback registers the callback function that will be called when WriteArmaCallback is called
//
//export RVExtensionRegisterCallback
func RVExtensionRegisterCallback(fnc C.extensionCallback) {
	extensionCallbackFnc = fnc
}

// runExtensionCallback calls the callback function
func runExtensionCallback(name *C.char, function *C.char, data *C.char) C.int {
	return C.runExtensionCallback(extensionCallbackFnc, name, function, data)
}

// WriteArmaCallback takes a function name designation and a series of arguments that it will parse into an array and send to Arma
func WriteArmaCallback(
	extensionName string,
	functionName string,
	data ...string,
) (
	err error,
) {

	// preprocess data with escape characters
	for i, v := range data {
		// replace double quotes with 2 double quotes
		escapedData := strings.ReplaceAll(v, `"`, `""`)
		// do the same for single quotes
		escapedData = strings.ReplaceAll(escapedData, `'`, `''`)
		// replace brackets w parentheses
		escapedData = strings.ReplaceAll(escapedData, `[`, `(`)
		escapedData = strings.ReplaceAll(escapedData, `]`, `)`)

		data[i] = fmt.Sprintf(`"%s"`, escapedData)
	}
	// format the data into a string
	a3Message := fmt.Sprintf(`[%s]`, strings.Join(data, ","))

	// Test hook: divert to an in-process sink if one was installed.
	callbackSinkMu.Lock()
	sink := callbackSink
	callbackSinkMu.Unlock()
	if sink != nil {
		sink(extensionName, functionName, data...)
		return nil
	}

	// check if the callback function is set
	if extensionCallbackFnc != nil {
		statusName := C.CString(extensionName)
		defer C.free(unsafe.Pointer(statusName))
		statusFunction := C.CString(functionName)
		defer C.free(unsafe.Pointer(statusFunction))
		statusParam := C.CString(a3Message)
		defer C.free(unsafe.Pointer(statusParam))
		// call the callback function
		runExtensionCallback(statusName, statusFunction, statusParam)
		return nil
	}
	return fmt.Errorf("callback function not set")
}
