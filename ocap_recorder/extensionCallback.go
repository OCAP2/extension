package main

/*
#include <stdlib.h>

typedef int (*extensionCallback)(char const *name, char const *function, char const *data);

// https://golang.org/cmd/cgo/#hdr-C_references_to_Go
static inline int runExtensionCallback(extensionCallback fnc, char const *name, char const *function, char const *data)
{
	return fnc(name, function, data);
}
*/
import "C"

var extensionCallbackFnc C.extensionCallback

//export RVExtensionRegisterCallback
func RVExtensionRegisterCallback(fnc C.extensionCallback) {
	extensionCallbackFnc = fnc
}

func runExtensionCallback(name *C.char, function *C.char, data *C.char) C.int {
	return C.runExtensionCallback(extensionCallbackFnc, name, function, data)
}
