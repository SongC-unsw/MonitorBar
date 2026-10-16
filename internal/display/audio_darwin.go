//go:build darwin

package display

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// CoreAudio tells where the sound goes: its default output device, and
// how that is connected.
var (
	audioObjectGetPropertyData func(object uint32, address *audioAddress, qualifierSize uint32, qualifier unsafe.Pointer, size *uint32, data unsafe.Pointer) int32
	audioOnce                  sync.Once
	audioOK                    bool
)

// audioAddress is AudioObjectPropertyAddress.
type audioAddress struct{ selector, scope, element uint32 }

const (
	audioSystemObject      = 1
	audioDefaultOutput     = 0x644F7574 // 'dOut'
	audioTransportType     = 0x7472616E // 'tran'
	audioScopeGlobal       = 0x676C6F62 // 'glob'
	audioTransportHDMI     = 0x68646D69 // 'hdmi'
	audioTransportDP       = 0x64707274 // 'dprt'
	audioPropertyMainValue = 0
)

// SoundToDisplay reports whether the Mac's sound goes to a display, over
// HDMI or DisplayPort: macOS has no volume for that, the monitor does.
func SoundToDisplay() bool {
	audioOnce.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/CoreAudio.framework/CoreAudio", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&audioObjectGetPropertyData, lib, "AudioObjectGetPropertyData")
		audioOK = true
	})
	if !audioOK {
		return false
	}
	get := func(object, selector uint32) (uint32, bool) {
		address := audioAddress{selector, audioScopeGlobal, audioPropertyMainValue}
		var v uint32
		size := uint32(unsafe.Sizeof(v))
		return v, audioObjectGetPropertyData(object, &address, 0, nil, &size, unsafe.Pointer(&v)) == 0
	}
	device, ok := get(audioSystemObject, audioDefaultOutput)
	if !ok || device == 0 {
		return false
	}
	transport, ok := get(device, audioTransportType)
	return ok && (transport == audioTransportHDMI || transport == audioTransportDP)
}
