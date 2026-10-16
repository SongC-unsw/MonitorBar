//go:build darwin

package mediakeys

import (
	"errors"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// SettingsURL opens the list of apps that may control the computer, where
// the user lets the app take the keys.
const SettingsURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"

const (
	sessionEventTap   = 1          // kCGSessionEventTap
	headInsert        = 0          // kCGHeadInsertEventTap
	tapDefault        = 0          // kCGEventTapOptionDefault: it may keep events from the system
	systemDefined     = 14         // NX_SYSDEFINED, the media keys' events
	auxControlButtons = 8          // their subtype
	tapTimedOut       = 0xFFFFFFFE // kCGEventTapDisabledByTimeout
	tapUserDisabled   = 0xFFFFFFFF // kCGEventTapDisabledByUserInput
)

var (
	cgEventTapCreate              func(tap, place, options uint32, mask uint64, callback, info uintptr) uintptr
	cgEventTapEnable              func(tap uintptr, enable bool)
	cgEventGetFlags               func(event uintptr) uint64
	cfMachPortCreateRunLoopSource func(alloc, port uintptr, order int) uintptr
	cfMachPortInvalidate          func(port uintptr)
	cfRunLoopGetMain              func() uintptr
	cfRunLoopAddSource            func(loop, source, mode uintptr)
	cfRunLoopRemoveSource         func(loop, source, mode uintptr)
	cfRelease                     func(ref uintptr)
	cfDictionaryCreate            func(alloc uintptr, keys, values *uintptr, n int, keyCallbacks, valueCallbacks uintptr) uintptr
	axIsProcessTrustedWithOptions func(options uintptr) bool
	autoreleasePoolPush           func() uintptr
	autoreleasePoolPop            func(pool uintptr)

	commonModes  uintptr // kCFRunLoopCommonModes
	promptOption uintptr // {kAXTrustedCheckOptionPrompt: true}
	callback     uintptr // tapped, for CoreGraphics

	nsEvent                                   objc.Class
	selEventWithCGEvent, selSubtype, selData1 objc.SEL

	loadOnce sync.Once
	loadErr  error

	current *tap // the tap the callback serves, on the main thread
)

func load() error {
	loadOnce.Do(func() {
		lib := func(path string) uintptr {
			h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				loadErr = err
			}
			return h
		}
		cg := lib("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
		cf := lib("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
		as := lib("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices")
		libobjc := lib("/usr/lib/libobjc.A.dylib")
		lib("/System/Library/Frameworks/AppKit.framework/AppKit")
		if loadErr != nil {
			return
		}
		for fn, name := range map[any]string{
			&cgEventTapCreate: "CGEventTapCreate", &cgEventTapEnable: "CGEventTapEnable", &cgEventGetFlags: "CGEventGetFlags",
		} {
			purego.RegisterLibFunc(fn, cg, name)
		}
		for fn, name := range map[any]string{
			&cfMachPortCreateRunLoopSource: "CFMachPortCreateRunLoopSource", &cfMachPortInvalidate: "CFMachPortInvalidate",
			&cfRunLoopGetMain: "CFRunLoopGetMain", &cfRunLoopAddSource: "CFRunLoopAddSource",
			&cfRunLoopRemoveSource: "CFRunLoopRemoveSource", &cfRelease: "CFRelease", &cfDictionaryCreate: "CFDictionaryCreate",
		} {
			purego.RegisterLibFunc(fn, cf, name)
		}
		purego.RegisterLibFunc(&axIsProcessTrustedWithOptions, as, "AXIsProcessTrustedWithOptions")
		purego.RegisterLibFunc(&autoreleasePoolPush, libobjc, "objc_autoreleasePoolPush")
		purego.RegisterLibFunc(&autoreleasePoolPop, libobjc, "objc_autoreleasePoolPop")

		// value reads a global pointer variable such as kCFBooleanTrue.
		value := func(lib uintptr, name string) uintptr {
			addr, err := purego.Dlsym(lib, name)
			if err != nil {
				loadErr = err
				return 0
			}
			return **(**uintptr)(unsafe.Pointer(&addr))
		}
		commonModes = value(cf, "kCFRunLoopCommonModes")
		key, yes := value(as, "kAXTrustedCheckOptionPrompt"), value(cf, "kCFBooleanTrue")
		if loadErr != nil {
			return
		}
		keyCallbacks, _ := purego.Dlsym(cf, "kCFTypeDictionaryKeyCallBacks")
		valueCallbacks, _ := purego.Dlsym(cf, "kCFTypeDictionaryValueCallBacks")
		promptOption = cfDictionaryCreate(0, &key, &yes, 1, keyCallbacks, valueCallbacks)

		nsEvent = objc.GetClass("NSEvent")
		selEventWithCGEvent, selSubtype, selData1 = objc.RegisterName("eventWithCGEvent:"), objc.RegisterName("subtype"), objc.RegisterName("data1")
		callback = purego.NewCallback(tapped)
	})
	return loadErr
}

// Trusted reports whether the user lets the app take the keys; with
// prompt, macOS asks them to, unless they did.
func Trusted(prompt bool) bool {
	if load() != nil {
		return false
	}
	var options uintptr
	if prompt {
		options = promptOption
	}
	return axIsProcessTrustedWithOptions(options)
}

// tap is an event tap that sees the media keys.
type tap struct {
	port, source uintptr
	keys         keys
}

// Tap hands each press of the brightness and volume keys to handle, on the
// main thread, until stop is called: a press it takes is kept from macOS,
// with the key's release. Call it on the main thread.
func Tap(handle func(Press) bool) (stop func(), err error) {
	if err := load(); err != nil {
		return nil, err
	}
	if !Trusted(false) {
		return nil, ErrNotTrusted
	}
	port := cgEventTapCreate(sessionEventTap, headInsert, tapDefault, 1<<systemDefined, callback, 0)
	if port == 0 {
		return nil, errors.New("mediakeys: macOS refused the event tap")
	}
	t := &tap{port: port, source: cfMachPortCreateRunLoopSource(0, port, 0), keys: keys{handle: handle, held: map[Key]bool{}}}
	cfRunLoopAddSource(cfRunLoopGetMain(), t.source, commonModes)
	cgEventTapEnable(port, true)
	current = t
	var once sync.Once
	return func() {
		once.Do(func() {
			if current == t {
				current = nil
			}
			cgEventTapEnable(t.port, false)
			cfRunLoopRemoveSource(cfRunLoopGetMain(), t.source, commonModes)
			cfMachPortInvalidate(t.port)
			cfRelease(t.source)
			cfRelease(t.port)
		})
	}, nil
}

// tapped is the callback of the event tap, on the main thread: it returns
// the event to pass it on, or 0 to keep it from macOS.
func tapped(proxy, typ, event, info uintptr) uintptr {
	t := current
	switch uint32(typ) {
	case tapTimedOut, tapUserDisabled:
		// macOS turns off a tap that was slow to answer: turn it on again.
		if t != nil {
			cgEventTapEnable(t.port, true)
		}
		return event
	case systemDefined:
	default:
		return event
	}
	if t == nil {
		return event
	}
	pool := autoreleasePoolPush()
	defer autoreleasePoolPop(pool)
	ns := objc.ID(nsEvent).Send(selEventWithCGEvent, event)
	if ns == 0 || objc.Send[int16](ns, selSubtype) != auxControlButtons {
		return event
	}
	if t.keys.take(decode(objc.Send[int](ns, selData1), cgEventGetFlags(event))) {
		return 0
	}
	return event
}
