package main

import (
	"runtime"

	"github.com/ebitengine/purego/objc"
)

// MyGo v0.7.3 reads AppleFontSmoothing when initializing Core Text. Override
// it in the process's volatile argument domain, equivalent to the reference
// app's -webkit-font-smoothing: antialiased. No preferences are written to disk
// and no other application's font rendering is changed.
func configureReaderTextRendering() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	str := func(s string) objc.ID {
		return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), s)
	}
	defaults := objc.ID(objc.GetClass("NSUserDefaults")).Send(objc.RegisterName("standardUserDefaults"))
	domain := str("NSArgumentDomain")
	values := defaults.Send(objc.RegisterName("volatileDomainForName:"), domain).Send(objc.RegisterName("mutableCopy"))
	if values == 0 {
		values = objc.ID(objc.GetClass("NSMutableDictionary")).Send(objc.RegisterName("new"))
	}
	defer values.Send(objc.RegisterName("release"))
	zero := objc.ID(objc.GetClass("NSNumber")).Send(objc.RegisterName("numberWithInt:"), int32(0))
	values.Send(objc.RegisterName("setObject:forKey:"), zero, str("AppleFontSmoothing"))
	defaults.Send(objc.RegisterName("setVolatileDomain:forName:"), values, domain)
}
