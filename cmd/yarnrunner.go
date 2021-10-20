//go:build example
// +build example

// Copyright 2021 Josh Deprez
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

	"github.com/DrJosh9000/yarn"
	yarnpb "github.com/DrJosh9000/yarn/bytecode"
	"google.golang.org/protobuf/proto"
)

func main() {
	yarncFilename := flag.String("program", "", "File name of program (e.g. Example.yarn.yarnc)")
	csvFilename := flag.String("strings", "", "File name of string table (e.g. Example.yarn.csv)")
	startNode := flag.String("start", "Start", "Name of the node to run")
	langCode := flag.String("lang", "en-AU", "Language code")
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
	stringTable, err := yarn.ReadStringTable(csv, *langCode)
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
	h.virtualMachine = vm

	if err := vm.Run(*startNode); err != nil {
		log.Printf("Yarn VM error: %v", err)
	}
}

// dialogueHandler implements yarn.DialogueHandler by playing the lines and
// options on the terminal.
type dialogueHandler struct {
	stringTable    *yarn.StringTable
	virtualMachine *yarn.VirtualMachine
}

func (h *dialogueHandler) Line(line yarn.Line) error {
	text, err := h.stringTable.Render(line)
	if err != nil {
		return err
	}
	fmt.Println(text)
	fmt.Print("(Press ENTER to continue)")
	fmt.Scanln()
	// This next string is VT100 for "move to the first column, go up a line,
	// and erase it" (erasing the Press ENTER message).
	fmt.Print("\r\033[A\033[2K")
	return nil
}

func (h *dialogueHandler) Options(opts []yarn.Option) (int, error) {
	fmt.Println("Choose:")
	for _, opt := range opts {
		text, err := h.stringTable.Render(opt.Line)
		if err != nil {
			return 0, err
		}
		fmt.Printf("%d: %s\n", opt.ID, text)
	}
	var choice int
	for {
		fmt.Print("Enter the number corresponding to your choice: ")
		if _, err := fmt.Scanln(&choice); err != nil {
			continue
		}
		break
	}
	return choice, nil
}

// Don't care about any of these:

func (h *dialogueHandler) Command(string) error           { return nil }
func (h *dialogueHandler) NodeStart(string) error         { return nil }
func (h *dialogueHandler) PrepareForLines([]string) error { return nil }
func (h *dialogueHandler) NodeComplete(string) error      { return nil }
func (h *dialogueHandler) DialogueComplete() error        { return nil }
