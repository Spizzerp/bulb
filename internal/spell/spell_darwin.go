//go:build darwin && !ios

package spell

import (
	"runtime"
	"strings"

	"github.com/ebitengine/purego"
)

const (
	appKitPath  = "/System/Library/Frameworks/AppKit.framework/AppKit"
	libObjCPath = "/usr/lib/libobjc.A.dylib"
)

// nsRange is Foundation's NSRange; NSNotFound is its location when a search
// finds nothing.
type nsRange struct{ loc, length uint64 }

const nsNotFound = 1<<63 - 1

// appKitChecker asks NSSpellChecker, which knows the user's languages and
// the words they taught it.
type appKitChecker struct {
	send     func(self, sel uintptr) uintptr
	sendStr  func(self, sel uintptr, s *byte) uintptr
	sendSpan func(self, sel, str uintptr, start int64) nsRange

	checker, stringClass, poolClass         uintptr
	selCheck, selFromUTF8, selNew, selDrain uintptr
}

func newPlatformChecker() checker {
	objc, err := purego.Dlopen(libObjCPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil
	}
	if _, err := purego.Dlopen(appKitPath, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return nil
	}
	msgSend, err := purego.Dlsym(objc, "objc_msgSend")
	if err != nil {
		return nil
	}
	c := &appKitChecker{}
	purego.RegisterFunc(&c.send, msgSend)
	purego.RegisterFunc(&c.sendStr, msgSend)
	purego.RegisterFunc(&c.sendSpan, msgSend)
	var getClass, registerName func(string) uintptr
	purego.RegisterLibFunc(&getClass, objc, "objc_getClass")
	purego.RegisterLibFunc(&registerName, objc, "sel_registerName")

	checkerClass := getClass("NSSpellChecker")
	c.stringClass = getClass("NSString")
	c.poolClass = getClass("NSAutoreleasePool")
	if checkerClass == 0 || c.stringClass == 0 || c.poolClass == 0 {
		return nil
	}
	c.selCheck = registerName("checkSpellingOfString:startingAt:")
	c.selFromUTF8 = registerName("stringWithUTF8String:")
	c.selNew = registerName("new")
	c.selDrain = registerName("drain")

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := c.send(c.poolClass, c.selNew)
	defer c.send(pool, c.selDrain)
	c.checker = c.send(checkerClass, registerName("sharedSpellChecker"))
	if c.checker == 0 {
		return nil
	}
	return c
}

// misspelled checks every word in one call: the words are joined with spaces
// and each miss NSSpellChecker reports is mapped back to the word it falls in.
// The words are ASCII-free of spaces by construction (spell.Words), and
// NSString offsets are UTF-16 units, so offsets are converted per word.
func (c *appKitChecker) misspelled(words []string) (map[string]bool, bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := c.send(c.poolClass, c.selNew)
	defer c.send(pool, c.selDrain)

	text := strings.Join(words, " ")
	cstr := append([]byte(text), 0)
	str := c.sendStr(c.stringClass, c.selFromUTF8, &cstr[0])
	runtime.KeepAlive(cstr)
	if str == 0 {
		return nil, false
	}
	// starts[i] is the UTF-16 offset of words[i] in text.
	starts := make([]uint64, len(words))
	var off uint64
	for i, w := range words {
		starts[i] = off
		off += utf16Len(w) + 1
	}
	bad := map[string]bool{}
	var at uint64
	for at < off {
		r := c.sendSpan(c.checker, c.selCheck, str, int64(at))
		// The search wraps to the start once it passes the end: a miss before
		// where it started means every miss has been seen.
		if r.length == 0 || r.loc == nsNotFound || r.loc < at {
			break
		}
		if i := wordAt(starts, r.loc); i >= 0 {
			bad[words[i]] = true
		}
		at = r.loc + r.length
	}
	return bad, true
}

// wordAt returns the index of the word whose span starts at or before the
// UTF-16 offset loc, or -1.
func wordAt(starts []uint64, loc uint64) int {
	i := -1
	for j, s := range starts {
		if s > loc {
			break
		}
		i = j
	}
	return i
}

func utf16Len(s string) uint64 {
	var n uint64
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}
