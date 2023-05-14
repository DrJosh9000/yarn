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

// The yarndumper binary prints the program in a pseudo-assembler format.
//
// Quick usage from the root of the repo:
//
//	go run -tags example cmd/yarndumper/yarndumper.go testdata/Example.yarn.yarnc
//
// The "example" build tag is used to prevent this being installed to ~/go/bin
// if you use the go get command. If for some reason you want to install it to
// your ~/go/bin, use `go install -tags example cmd/yarndumper.go` or similar.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/DrJosh9000/yarn"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprint(os.Stderr, "Usage: yarndumper YARNC_FILE")
		os.Exit(1)
	}
	program, err := yarn.LoadProgramFile(os.Args[1])
	if err != nil {
		log.Fatalf("Couldn't read program file: %v", err)
	}
	yarn.FormatProgram(os.Stdout, program)
}
