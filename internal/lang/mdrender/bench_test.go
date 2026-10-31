package mdrender

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rivo/tview"
)

// genBenchMarkdown produces realistic markdown (heading + prose-paragraph
// sections, repeated) at approximately the given size — the shape found to
// matter: ToTviewTags' cost scales with total document size, not with
// content on screen (see BACKLOG.md's "Markdown preview isn't
// viewport-scoped" entry, raised directly against this project's "smooth
// and light, even on huge files" pillar).
func genBenchMarkdown(approxBytes int) string {
	var b strings.Builder
	const section = "## Heading %d\n\nSome paragraph text that repeats a bunch of words to pad out the line length realistically, section number %d, more words more words more words.\n\n"
	for i := 0; b.Len() < approxBytes; i++ {
		fmt.Fprintf(&b, section, i, i)
	}
	return b.String()
}

// BenchmarkToTviewTagsOnLargeDocument is run via `task bench`, not the
// normal test suite (Go only runs Benchmark* functions with -bench) — a
// standing, repeatable way to re-check these numbers instead of
// re-deriving them with a throwaway script each time scaling comes up.
// Sub-benchmarks cover 1MB and 10MB; see BACKLOG.md's entry for 50MB/200MB
// numbers from the one-off run that first surfaced this (too slow for a
// benchmark that runs routinely).
func BenchmarkToTviewTagsOnLargeDocument(b *testing.B) {
	for _, mb := range []int{1, 10} {
		src := []byte(genBenchMarkdown(mb * 1024 * 1024))
		b.Run(fmt.Sprintf("%dMB", mb), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ToTviewTags(src)
			}
		})
	}
}

// BenchmarkVisualRowWrapOnLargeDocument covers the second half of the same
// cost — tui-base's own word-wrap pass over ToTviewTags' output (see
// visualRowLineMap in internal/frontend/tui-base/base.go), measured here
// directly against tview.WordWrap since mdrender has no frontend
// dependency to import tui-base's own helper.
func BenchmarkVisualRowWrapOnLargeDocument(b *testing.B) {
	for _, mb := range []int{1, 10} {
		rendered, _ := ToTviewTags([]byte(genBenchMarkdown(mb * 1024 * 1024)))
		lines := strings.Split(rendered, "\n")
		b.Run(fmt.Sprintf("%dMB", mb), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, line := range lines {
					tview.WordWrap(line, 100)
				}
			}
		})
	}
}
