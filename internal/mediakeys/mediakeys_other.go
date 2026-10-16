//go:build !darwin

package mediakeys

import "errors"

// SettingsURL is where the user lets the app take the keys: nowhere, as
// only macOS has them taken.
const SettingsURL = ""

// Trusted reports false: the keys are taken on macOS only.
func Trusted(prompt bool) bool { return false }

// Tap takes the keys on macOS only.
func Tap(handle func(Press) bool) (stop func(), err error) { return nil, errors.ErrUnsupported }
