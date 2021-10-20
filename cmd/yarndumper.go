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
//    go run -tags example cmd/yarndumper.go \
//        --program=testdata/Example.yarn.yarnc \
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/DrJosh9000/yarn"
	yarnpb "github.com/DrJosh9000/yarn/bytecode"
	"google.golang.org/protobuf/proto"
)

func main() {
	yarncFilename := flag.String("program", "", "File name of program (e.g. Example.yarn.yarnc)")
	flag.Parse()

	yarnc, err := ioutil.ReadFile(*yarncFilename)
	if err != nil {
		log.Fatalf("Couldn't read program file: %v", err)
	}
	program := new(yarnpb.Program)
	if err := proto.Unmarshal(yarnc, program); err != nil {
		log.Fatalf("Couldn't unmarshal program: %v", err)
	}

	fmt.Println(yarn.FormatProgram(program))
}
