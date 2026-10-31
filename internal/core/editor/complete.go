package editor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// commandNames lists every ":..." command this app recognizes anywhere —
// not only executeCommandLocked's own switch, but also commands
// intercepted a layer up before ever reaching it ("edit" by
// *workspace.Workspace, "MdPreview" by tui-base itself) — each by its
// longest/clearest spelling (sorted, case-sensitive: the LspXxx family's
// capital L sorts before the lowercase ones). This is the set command-name
// completion (T27, below) cycles through while the command buffer still
// has no space in it; a user typing ":<Tab>" has no reason to care which
// layer ends up handling the command, so this list intentionally isn't
// scoped to just this package's own switch. Deliberately not derived from
// any single switch (that would need reflection or a codegen step
// unwarranted for a dozen literal names, and couldn't reach across
// packages anyway) — keep this in sync by hand whenever a command is added
// anywhere, same discipline exmode.Parse already assumes of its own
// callers.
var commandNames = []string{
	"LspInstall",
	"LspStatus",
	"LspUninstall",
	"MdPreview",
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
