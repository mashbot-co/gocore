package connection

import (
	"testing"

	"gorm.io/gorm"
)

// TestRegisterCallbacksOn verifies the exported wrapper runs every registered
// callback against the given DB. callbackFns is saved/restored so this doesn't
// perturb other tests in the package.
func TestRegisterCallbacksOn(t *testing.T) {
	saved := callbackFns
	t.Cleanup(func() { callbackFns = saved })
	callbackFns = nil

	var got *gorm.DB
	OnInitialize(func(db *gorm.DB) { got = db })

	want := &gorm.DB{}
	RegisterCallbacksOn(want)

	if got != want {
		t.Error("RegisterCallbacksOn did not invoke the registered callback with the provided DB")
	}
}
