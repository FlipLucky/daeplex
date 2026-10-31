package viewmanager

import "testing"

func TestGutterWidthGrowsWithRowCount(t *testing.T) {
	cases := []struct {
		highestVisibleRow int
		want              int
	}{
		{0, 4},     // 1 digit, floored to 3 + 1 space
		{9, 4},     // row 9 -> line 10, still 2 digits, floored to 3 + 1
		{99, 4},    // line 100, 3 digits + 1
		{999, 5},   // line 1000, 4 digits + 1
		{99998, 6}, // line 99999, 5 digits + 1
	}
	for _, c := range cases {
		if got := GutterWidth(c.highestVisibleRow); got != c.want {
			t.Errorf("GutterWidth(%d) = %d, want %d", c.highestVisibleRow, got, c.want)
		}
	}
}

func TestGutterTextOffWhenNeitherOptionSet(t *testing.T) {
	if got := GutterText(5, 5, false, false, 4); got != "" {
		t.Errorf("GutterText with both options off = %q, want empty", got)
	}
}

func TestGutterTextAbsoluteOnly(t *testing.T) {
	// number only, no relativenumber: every row (including the cursor's
	// own) shows its real 1-indexed line number, right-aligned.
	if got := GutterText(0, 0, true, false, 4); got != "  1 " {
		t.Errorf("current line, number only = %q, want %q", got, "  1 ")
	}
	if got := GutterText(9, 0, true, false, 4); got != " 10 " {
		t.Errorf("other line, number only = %q, want %q", got, " 10 ")
	}
}

func TestGutterTextRelativeOnlyShowsZeroOnCurrentLine(t *testing.T) {
	if got := GutterText(5, 5, false, true, 4); got != "0   " {
		t.Errorf("current line, relative only = %q, want %q", got, "0   ")
	}
	if got := GutterText(8, 5, false, true, 4); got != "  3 " {
		t.Errorf("3 lines below cursor = %q, want %q", got, "  3 ")
	}
	if got := GutterText(2, 5, false, true, 4); got != "  3 " {
		t.Errorf("3 lines above cursor = %q, want %q", got, "  3 ")
	}
}

func TestGutterTextHybridShowsAbsoluteOnCurrentLine(t *testing.T) {
	// number + relativenumber together (the real, confirmed-via-dotfiles
	// "hybrid" habit): the current line shows its own absolute number,
	// left-aligned (vim's own convention, distinct from every relative
	// row around it); every other line shows relative distance.
	if got := GutterText(5, 5, true, true, 4); got != "6   " {
		t.Errorf("current line, hybrid = %q, want %q", got, "6   ")
	}
	if got := GutterText(8, 5, true, true, 4); got != "  3 " {
		t.Errorf("3 lines below cursor, hybrid = %q, want %q", got, "  3 ")
	}
}
