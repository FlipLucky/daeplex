package main

import (
	"log"
	"os"

	"github.com/fliplucky/daeplex/internal/core/editor"
	"github.com/fliplucky/daeplex/internal/core/workspace"
	tuibase "github.com/fliplucky/daeplex/internal/frontend/tui-base"
)

func main() {
	var ed *editor.Editor
	var err error

	if len(os.Args) > 1 {
		ed, err = editor.NewEditorFromFile(os.Args[1])
		if err != nil {
			log.Fatalf("failed to open file %s: %v", os.Args[1], err)
		}
	} else {
		ed = editor.NewEditor("")
	}

	if err := tuibase.Boot(workspace.New(ed)); err != nil {
		log.Fatalf("failed to boot TUI: %v", err)
	}
}
