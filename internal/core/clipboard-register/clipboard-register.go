package clipboardregister

import "sync"

type RegisterType int

// Entry is one stored register value together with whether it came from a
// linewise operation (yy/dd/cc — a whole line, via keyengine's generic
// doubled-verb/LineMotion mechanism) or a charwise one (yiw/diw/ciw and
// friends — an arbitrary span of text). Paste needs this to decide whether
// pasted text becomes its own fresh line or splices in at the cursor the
// way a plain text insert does.
//
// Values has length 1 for every charwise/linewise entry (Values[0] is the
// whole stored text) and length N for a block-visual entry (T54,
// 2026-10-18) — one string per row spanned, sharing this one field rather
// than a second, parallel shape (Entry.Value string was widened to this
// only once T54 actually needed a second value per entry — T52's own
// char/linewise work deliberately left it alone, see ATOMIC_TASKS.md's
// T52 writeup). len(Values) > 1 is what Paste/:registers use to tell a
// block entry apart from a charwise/linewise one — no separate flag.
type Entry struct {
	Values   []string
	Linewise bool
}

type Register []Entry

const (
	Unnamed RegisterType = iota
	Yank
	Delete
	Visual
)

// ClipboardRegister is workspace-global as of T57 (2026-10-18) — one
// instance shared by every open buffer (see internal/workspace), not one
// per Editor — matching real vim's own registers, which are never
// per-buffer. Every exported method below self-locks via mu; no
// unexported helper (getRegister, appendToRegister) ever locks, and none
// should be called except already-locked, same convention as
// internal/piecetable's own locking invariant.
type ClipboardRegister struct {
	mu              sync.Mutex
	unnamedRegister Register
	namedRegister   map[rune]Entry
	yankRegister    Register
	visualRegister  Register
	deleteRegister  Register
}

func (c *ClipboardRegister) getRegister(
	register RegisterType,
) *Register {
	switch register {
	case Yank:
		return &c.yankRegister
	case Delete:
		return &c.deleteRegister
	case Visual:
		return &c.visualRegister
	}
	return nil
}

// StoreRegisterValue always updates the unnamed register (every delete/
// yank is pastable with a bare p/P, matching real vim) and the category
// register for registerType. named is additionally the user-selected
// register letter from a "{letter} prefix (e.g. "ayw stores into 'a' too);
// 0 means no named register was selected for this command.
func (c *ClipboardRegister) StoreRegisterValue(
	value string,
	registerType RegisterType,
	linewise bool,
	named rune,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := Entry{Values: []string{value}, Linewise: linewise}
	appendToRegister(entry, &c.unnamedRegister)
	if register := c.getRegister(registerType); register != nil {
		appendToRegister(entry, register)
	}
	if named != 0 {
		if c.namedRegister == nil {
			c.namedRegister = make(map[rune]Entry)
		}
		c.namedRegister[named] = entry
	}
}

// StoreBlockRegisterValue is StoreRegisterValue's block-mode sibling (T54,
// 2026-10-18): values holds one string per row spanned by a block-visual
// selection, stored as a single multi-value Entry rather than N separate
// ones. Otherwise identical — always updates the unnamed register too,
// plus the given category register and, if set, a named letter register.
func (c *ClipboardRegister) StoreBlockRegisterValue(
	values []string,
	registerType RegisterType,
	named rune,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := Entry{Values: values}
	appendToRegister(entry, &c.unnamedRegister)
	if register := c.getRegister(registerType); register != nil {
		appendToRegister(entry, register)
	}
	if named != 0 {
		if c.namedRegister == nil {
			c.namedRegister = make(map[rune]Entry)
		}
		c.namedRegister[named] = entry
	}
}

func appendToRegister(
	entry Entry,
	register *Register,
) {
	*register = append(Register{entry}, (*register)...)
	if len(*register) > 9 {
		*register = (*register)[:9]
	}
}

func (c *ClipboardRegister) GetValueFromRegister(
	registerType RegisterType, index int,
) (Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	register := c.getRegister(registerType)
	if register == nil || (index < 0 || index >= len(*register)) {
		return Entry{}, false
	}
	return (*register)[index], true
}

func (c *ClipboardRegister) GetValueFromUnnamedRegister(
	index int,
) (Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getValueFromUnnamedRegister(index)
}

// getValueFromUnnamedRegister is GetValueFromUnnamedRegister's lock-free
// body — split out so Paste can share it without calling back into an
// already-locking exported method (sync.Mutex isn't reentrant; that would
// deadlock on first use).
func (c *ClipboardRegister) getValueFromUnnamedRegister(index int) (Entry, bool) {
	if index < 0 || index >= len(c.unnamedRegister) {
		return Entry{}, false
	}
	return c.unnamedRegister[index], true
}

func (c *ClipboardRegister) Paste() (Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getValueFromUnnamedRegister(0)
}

// GetValueFromNamedRegister looks up a single named register by its
// letter — single-value, no history, unlike the category registers
// (matches real vim's own a-z registers; this project deliberately
// skips the uppercase-append convention, lowercase only).
func (c *ClipboardRegister) GetValueFromNamedRegister(name rune) (Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.namedRegister[name]
	return entry, ok
}

// TopEntries returns up to n most-recent entries for registerType, most
// recent first — the register overview's (":register") read path.
// Unnamed is special-cased since getRegister doesn't resolve it (it's
// always updated directly, never routed through the splitter).
func (c *ClipboardRegister) TopEntries(registerType RegisterType, n int) []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	var register *Register
	if registerType == Unnamed {
		register = &c.unnamedRegister
	} else {
		register = c.getRegister(registerType)
	}
	if register == nil {
		return nil
	}
	if n > len(*register) {
		n = len(*register)
	}
	return (*register)[:n]
}

// NamedEntries returns every currently-set named register, keyed by
// letter — the register overview's read path for the a-z registers.
func (c *ClipboardRegister) NamedEntries() map[rune]Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.namedRegister
}
