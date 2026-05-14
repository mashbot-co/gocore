package connection

import "gorm.io/gorm"

// callbackFns holds functions to register on the GORM instance during Initialize.
// Packages like mixins register themselves via OnInitialize during init().
var callbackFns []func(*gorm.DB)

// OnInitialize registers a function to be called when the database is initialized.
// Use this from mixin packages to register GORM callbacks without circular imports.
func OnInitialize(fn func(*gorm.DB)) {
	callbackFns = append(callbackFns, fn)
}

func registerCallbacks(gormDB *gorm.DB) {
	for _, fn := range callbackFns {
		fn(gormDB)
	}
}

// RegisterCallbacksOn exposes the callback registration for use by other
// packages (e.g., testdb) that create their own GORM instances.
func RegisterCallbacksOn(gormDB *gorm.DB) {
	registerCallbacks(gormDB)
}
