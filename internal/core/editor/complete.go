package editor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// commandNames lists every command executeCommandLocked recognizes, each
// by its longest/clearest spelling (sorted, case-sensitive: the LspXxx
// family's capital L sorts before the lowercase ones) — the set
// command-name completion (T27, below) cycles through while the command
// buffer still has no space in it. Deliberately not derived from
// executeCommandLocked's own switch (that would need reflection or a
// codegen step unwarranted for a dozen literal names) — keep this in sync
// by hand when a command is added there, same discipline exmode.Parse
// already assumes of its own callers.
var commandNames = []string{
	"LspInstall",
	"LspStatus",
	"LspUninstall",
	"edit",
	"format",
	"quit",
	"quit!",
	"register",
	"registers",
	"set",
	"substitute",
	"wq",
	"write",
}

// commandNameCandidates returns every known command name with prefix as a
// literal (case-sensitive) prefix — an empty prefix matches everything,
// i.e. ":<Tab>" with nothing typed yet lists every command, same as real
// vim's own bare ":<Tab>" behavior.
func commandNameCandidates(prefix string) []string {
	var candidates []string
	for _, name := range commandNames {
		if strings.HasPrefix(name, prefix) {
			candidates = append(candidates, name)
		}
	}
	return candidates
}

// completionCandidates splits buffer at its last whitespace-separated
// token — the same "the last token might be a path" rule every
// command-mode command that takes one (:e, :w) shares — and returns
// everything before that token (lineHead, kept as-is) alongside the
// sorted list of filesystem entries matching it. No space yet (still
// typing the command name itself, e.g. ":Lsp<Tab>") completes against
// commandNames instead of falling through to a file listing.
func completionCandidates(buffer string) (lineHead string, candidates []string) {
	idx := strings.LastIndexByte(buffer, ' ')
	if idx < 0 {
		return "", commandNameCandidates(buffer)
	}
	lineHead, fragment := buffer[:idx+1], buffer[idx+1:]

	dir, namePrefix := filepath.Split(fragment)
	searchDir := dir
	if searchDir == "" {
		searchDir = "."
	}

	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return lineHead, nil
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), namePrefix) {
			continue
		}
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		candidates = append(candidates, dir+name)
	}
	sort.Strings(candidates)
	return lineHead, candidates
}
