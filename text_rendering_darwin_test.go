package main

import (
	"runtime"
	"testing"

	"github.com/ebitengine/purego/objc"
)

func TestReaderFontSmoothingIsProcessLocal(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	str := func(s string) objc.ID {
		return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), s)
	}
	defaults := objc.ID(objc.GetClass("NSUserDefaults")).Send(objc.RegisterName("standardUserDefaults"))
	if defaults == 0 {
		t.Fatal("NSUserDefaults unavailable")
	}
	domain := str("NSArgumentDomain")
	before := defaults.Send(objc.RegisterName("volatileDomainForName:"), domain).Send(objc.RegisterName("copy"))
	defer before.Send(objc.RegisterName("release"))
	defer defaults.Send(objc.RegisterName("setVolatileDomain:forName:"), before, domain)
	bundle := objc.ID(objc.GetClass("NSBundle")).Send(objc.RegisterName("mainBundle")).Send(objc.RegisterName("bundleIdentifier"))
	persistent := objc.ID(0)
	if bundle != 0 {
		persistent = defaults.Send(objc.RegisterName("persistentDomainForName:"), bundle)
	}
	configureReaderTextRendering()
	key := str("AppleFontSmoothing")
	value := defaults.Send(objc.RegisterName("volatileDomainForName:"), domain).Send(objc.RegisterName("objectForKey:"), key)
	if value == 0 || objc.Send[int](value, objc.RegisterName("intValue")) != 0 {
		t.Fatal("font smoothing override was not installed in the process domain")
	}
	if bundle != 0 {
		after := defaults.Send(objc.RegisterName("persistentDomainForName:"), bundle)
		if after != persistent && (persistent == 0 || !objc.Send[bool](persistent, objc.RegisterName("isEqual:"), after)) {
			t.Fatal("font smoothing changed persistent preferences")
		}
	}
}
