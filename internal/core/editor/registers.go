package editor

import (
	"fmt"
	"sort"
	"strings"

	clipboardregister "github.com/fliplucky/daeplex/internal/core/clipboard-register"
	"github.com/fliplucky/daeplex/internal/document/piecetable"
)

// maxRegisterPreviewLen caps a single register line's displayed content —
// same reasoning as gui-base's maxVirtualTextLen: a register can hold an
// arbitrarily large yank (a whole paragraph via yap), but the overview is
// meant to be a scannable summary, not a full reproduction.
const maxRegisterPreviewLen = 60

// maxEntriesPerCategory caps how many of a category register's history
// entries (yank/delete/visual each keep up to 9) the overview shows, per
// the user's own proposed layout: "top 3, unless empty."
const maxEntriesPerCategory = 3

// formatRegisterLine renders one register's content as a single display
// line, collapsing newlines and truncating by rune (not byte) — unlike
// gui-base/tui-base's per-frontend color mappings, this is pure string
// logic with zero toolkit dependency, so it's centralized here rather
// than duplicated per frontend.
func formatRegisterLine(name string, entry clipboardregister.Entry) string {
	kind := "c"
	switch {
	case len(entry.Values) > 1:
		kind = "b" // block-visual entry (T54) — len(Values) > 1 is the signal
	case entry.Linewise:
		kind = "l"
	}
	value := strings.ReplaceAll(strings.Join(entry.Values, "^J"), "\n", "^J")
	if runes := []rune(value); len(runes) > maxRegisterPreviewLen {
		value = string(runes[:maxRegisterPreviewLen-1]) + "…"
	}
	return fmt.Sprintf("%s  %-4s %s", kind, name, value)
}

// sortedRuneKeys returns m's keys in ascending order — named registers
// have no natural insertion order worth preserving, so the overview lists
// them alphabetically instead.
func sortedRuneKeys(m map[rune]clipboardregister.Entry) []rune {
	keys := make([]rune, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// openRegisterOverviewLocked builds the ":register"/":registers" display:
// unnamed first (if set), then up to maxEntriesPerCategory of each of
// yank/delete/visual (skipped entirely when empty), then every named
// register in letter order — the layout the user asked for directly,
// modeled after real nvim's own :register output but trimmed to stay
// scannable. Not a real Mode (see RegisterOverviewState's doc comment):
// it's dismissed by the next keystroke, same as Hover.
func (e *Editor) openRegisterOverviewLocked() {
	var lines []string

	if entries := e.clipboardRegister.TopEntries(clipboardregister.Unnamed, 1); len(entries) > 0 {
		lines = append(lines, formatRegisterLine(`""`, entries[0]))
	}

	categories := []struct {
		label string
		typ   clipboardregister.RegisterType
	}{
		{"yank", clipboardregister.Yank},
		{"delete", clipboardregister.Delete},
		{"visual", clipboardregister.Visual},
	}
	for _, cat := range categories {
		entries := e.clipboardRegister.TopEntries(cat.typ, maxEntriesPerCategory)
		for i, entry := range entries {
			lines = append(lines, formatRegisterLine(fmt.Sprintf("%s %d", cat.label, i), entry))
		}
	}

	named := e.clipboardRegister.NamedEntries()
	for _, name := range sortedRuneKeys(named) {
		lines = append(lines, formatRegisterLine(fmt.Sprintf("\"%c", name), named[name]))
	}

	if len(lines) == 0 {
		lines = []string{"(no registers)"}
	}

	e.cursor.RegisterOverview = RegisterOverviewState{Active: true, Lines: lines}
	e.notifyChange(piecetable.Edit{})
}

// dismissRegisterOverviewLocked closes the overview — called from
// handleNormalModeKey's dismiss-on-next-keystroke branch (dispatch.go),
// the same "transient tooltip" contract HoverState uses.
func (e *Editor) dismissRegisterOverviewLocked() {
	if e.cursor.RegisterOverview.Active {
		e.cursor.RegisterOverview = RegisterOverviewState{}
		e.notifyChange(piecetable.Edit{})
	}
}
