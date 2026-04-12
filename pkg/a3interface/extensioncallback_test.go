package a3interface

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteArmaCallback_DivertsToSink(t *testing.T) {
	var (
		mu      sync.Mutex
		gotName string
		gotFunc string
		gotData []string
	)

	SetCallbackSinkForTest(func(name, function string, data ...string) {
		mu.Lock()
		defer mu.Unlock()
		gotName = name
		gotFunc = function
		gotData = append(gotData, data...)
	})
	t.Cleanup(func() { SetCallbackSinkForTest(nil) })

	err := WriteArmaCallback("ocap_recorder", ":MISSION:SAVED:", "ok", "foo.json.gz")
	assert.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "ocap_recorder", gotName)
	assert.Equal(t, ":MISSION:SAVED:", gotFunc)
	assert.Len(t, gotData, 2)
	// data is pre-escaped and wrapped in quotes by WriteArmaCallback
	assert.Equal(t, `"ok"`, gotData[0])
	assert.Equal(t, `"foo.json.gz"`, gotData[1])
}

func TestWriteArmaCallback_NoSinkNoCallback(t *testing.T) {
	// No sink, no C callback registered → error
	SetCallbackSinkForTest(nil)
	extensionCallbackFnc = nil
	err := WriteArmaCallback("ocap_recorder", ":FOO:", "bar")
	assert.Error(t, err)
}
