package clipboardregister

import (
	"slices"
	"strconv"
	"sync"
	"testing"
)

// entriesEqual compares two Entry values by content — Entry.Values is now
// a slice (T54, 2026-10-18, for block mode's multi-row entries), so Entry
// is no longer comparable via == / slices.Equal directly.
func entriesEqual(a, b Entry) bool {
	return a.Linewise == b.Linewise && slices.Equal(a.Values, b.Values)
}

func TestAppendToRegisterPrependsNewest(t *testing.T) {
	reg := Register{{Values: []string{"old"}}}
	appendToRegister(Entry{Values: []string{"new"}}, &reg)

	want := Register{{Values: []string{"new"}}, {Values: []string{"old"}}}
	if !slices.EqualFunc(reg, want, entriesEqual) {
		t.Errorf("appendToRegister result = %v, want %v", reg, want)
	}
}

func TestAppendToVisualMode(t *testing.T) {
	cases := []struct {
		name         string
		initial      ClipboardRegister
		registerType RegisterType
		input        []string
		want         Register
	}{
		{
			name:         "test visual register",
			initial:      ClipboardRegister{},
			registerType: Visual,
			input:        []string{"first", "second", "third", "fourth"},
			want:         Register{{Values: []string{"fourth"}}, {Values: []string{"third"}}, {Values: []string{"second"}}, {Values: []string{"first"}}},
		},
		{
			name:         "test yank register",
			initial:      ClipboardRegister{},
			registerType: Yank,
			input:        []string{"first", "second", "third", "fourth"},
			want:         Register{{Values: []string{"fourth"}}, {Values: []string{"third"}}, {Values: []string{"second"}}, {Values: []string{"first"}}},
		},
		{
			name:         "test delete register",
			initial:      ClipboardRegister{},
			registerType: Delete,
			input:        []string{"first", "second", "third", "fourth"},
			want:         Register{{Values: []string{"fourth"}}, {Values: []string{"third"}}, {Values: []string{"second"}}, {Values: []string{"first"}}},
		},
	}
	// Indexed, not ranged-by-value: cases[i] is addressed in place so
	// ClipboardRegister's mutex (added T57) is never copied — a plain
	// "for _, c := range cases" would copy the whole case struct,
	// including the lock, which go vet's copylocks check rightly flags.
	for i := range cases {
		tc := &cases[i]
		for _, value := range tc.input {
			tc.initial.StoreRegisterValue(value, tc.registerType, false, 0)
		}
		register := tc.initial.getRegister(tc.registerType)
		if !slices.EqualFunc(tc.want, *register, entriesEqual) {
			t.Errorf("appendTo%v result = %v, want %v", tc.registerType, register, tc.want)
		}
	}

}

// TestConcurrentStoreAndRead guards the T57 (2026-10-18) sharing change —
// one ClipboardRegister is now held by every open buffer in a Workspace
// (internal/workspace), each driven by its own Editor's own goroutine-
// unsafe-by-itself call path, so real concurrent access across buffers is
// now possible where it never was before. Confirmed to actually fail with
// a real "WARNING: DATA RACE" under `go test -race` before mu was added
// (checked by temporarily reverting the lock), and clean after — same
// discipline as piecetable's own TestConcurrentInsertAndGetRuneAt (T16).
func TestConcurrentStoreAndRead(t *testing.T) {
	c := &ClipboardRegister{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			c.StoreRegisterValue("v"+strconv.Itoa(i), Yank, false, rune('a'+i%26))
		}(i)
		go func() {
			defer wg.Done()
			c.Paste()
			c.TopEntries(Yank, 3)
		}()
		go func(i int) {
			defer wg.Done()
			c.GetValueFromNamedRegister(rune('a' + i%26))
			c.NamedEntries()
		}(i)
	}
	wg.Wait()
}
