//go:build example
// +build example

// The yarnrunner binary runs a yarnc+string table combo as a "text game" on
// the terminal.
// The "example" build tag is used to prevent this being installed to go/bin
// if you use the go get command.
//
// Quick usage from the root of the repo:
//
//    go run -tags example cmd/yarnrunner.go \
//        --program=testdata/Example.yarn.yarnc \
//        --strings=testdata/Example.yarn.csv
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/DrJosh9000/yarn"
	yarnpb "github.com/DrJosh9000/yarn/bytecode"
	"google.golang.org/protobuf/proto"
)

func main() {
	yarncFilename := flag.String("program", "", "File name of program (e.g. Example.yarn.yarnc)")
	csvFilename := flag.String("strings", "", "File name of string table (e.g. Example.yarn.csv)")
	flag.Parse()

	yarnc, err := ioutil.ReadFile(*yarncFilename)
	if err != nil {
		log.Fatalf("Couldn't read program file: %v", err)
	}
	program := new(yarnpb.Program)
	if err := proto.Unmarshal(yarnc, program); err != nil {
		log.Fatalf("Couldn't unmarshal program: %v", err)
	}

	csv, err := os.Open(*csvFilename)
	if err != nil {
		log.Fatalf("Couldn't open string table file: %v", err)
	}
	defer csv.Close()
	stringTable, err := yarn.ReadStringTable(csv)
	if err != nil {
		log.Fatalf("Couldn't parse string table: %v", err)
	}

	h := &dialogueHandler{
		stringTable: stringTable,
	}
	vm := &yarn.VirtualMachine{
		Program: program,
		Handler: h,
		Vars:    make(yarn.MapVariableStorage),
	}
	h.vm = vm

	if err := vm.Run("Start"); err != nil {
		log.Printf("VirtualMachine exeption: %v", err)
	}
}

// dialogueHandler implements yarn.DialogueHandler by playing the lines and
// options on the terminal.
type dialogueHandler struct {
	stringTable yarn.StringTable
	vm          *yarn.VirtualMachine
}

func (h *dialogueHandler) Line(line yarn.Line) error {
	fmt.Println(h.stringTable[line.ID].Text)
	fmt.Println("\n(Press ENTER to continue)")
	fmt.Scanln()
	return nil
}

func (h *dialogueHandler) Options(opts []yarn.Option) (int, error) {
	fmt.Println("Choose:")
	for _, opt := range opts {
		fmt.Printf("%d: %s\n", opt.ID, h.stringTable[opt.Line.ID].Text)
	}
	fmt.Print("Enter the number corresponding to your choice: ")
	var choice int
	fmt.Scanf("%d", &choice)
	fmt.Println()
	return choice, nil
}

func (h *dialogueHandler) Command(command string) error {
	// Just implement "jump" for this example
	if strings.HasPrefix(command, "jump ") {
		return h.vm.SetNode(strings.TrimPrefix(command, "jump "))
	}
	return nil
}

// Don't care about any of these:

func (h *dialogueHandler) NodeStart(string) error         { return nil }
func (h *dialogueHandler) PrepareForLines([]string) error { return nil }
func (h *dialogueHandler) NodeComplete(string) error      { return nil }
func (h *dialogueHandler) DialogueComplete() error        { return nil }
