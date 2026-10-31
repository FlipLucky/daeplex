package main

import (
	"fmt"

	"github.com/fliplucky/daeplex/internal/document/piecetable"
)

func main() {
	table := piecetable.NewPieceTable([]byte("Hello World"))
	table.Insert(5, []byte(" Amazing"))
	fmt.Println(table.GetText())
	table.PrintDebug()
	table.Delete(2, 15)
	fmt.Println(table.GetText())
	table.PrintDebug()
}
