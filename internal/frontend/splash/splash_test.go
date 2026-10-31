package splash

import "testing"

func TestLinesHasExactlyOneAccentLineWithTheWordmark(t *testing.T) {
	lines := Lines()
	if len(lines) == 0 {
		t.Fatal("Lines() returned no content")
	}

	accentCount := 0
	for _, line := range lines {
		if line.Accent {
			accentCount++
			if line.Text != "DaePlex" {
				t.Errorf("accent line = %q, want %q", line.Text, "DaePlex")
			}
		}
	}
	if accentCount != 1 {
		t.Errorf("accent line count = %d, want exactly 1", accentCount)
	}
}
