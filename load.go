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

package yarn

import (
	"fmt"
	"io/fs"
	"os"

	yarnpb "github.com/kalexmills/yarn/bytecode"
	"google.golang.org/protobuf/proto"
)

// LoadFiles is a convenient way of loading a compiled Yarn Spinner program and
// string table from files in one function call. langCode should be a valid BCP
// 47 language tag.
func LoadFiles(programPath, stringTablePath, langCode string) (*yarnpb.Program, *StringTable, error) {
	prog, err := LoadProgramFile(programPath)
	if err != nil {
		return nil, nil, err
	}
	st, err := LoadStringTableFile(stringTablePath, langCode)
	if err != nil {
		return nil, nil, err
	}
	return prog, st, nil
}

// LoadFilesFS loads compiled Yarn Spinner files from the provided fs.FS.
// See LoadFiles for more information.
func LoadFilesFS(fsys fs.FS, programPath, stringTablePath, langCode string) (*yarnpb.Program, *StringTable, error) {
	yarnc, err := fs.ReadFile(fsys, programPath)
	if err != nil {
		return nil, nil, err
	}
	prog, err := unmarshalBytes(yarnc)
	if err != nil {
		return nil, nil, err
	}
	st, err := LoadStringTableFileFS(fsys, stringTablePath, langCode)
	if err != nil {
		return nil, nil, err
	}
	return prog, st, nil
}

// LoadProgramFile is a convenient function for loading a compiled Yarn Spinner
// program given a file path.
func LoadProgramFile(programPath string) (*yarnpb.Program, error) {
	yarnc, err := os.ReadFile(programPath)
	if err != nil {
		return nil, fmt.Errorf("reading program file: %w", err)
	}
	return unmarshalBytes(yarnc)
}

func unmarshalBytes(yarnc []byte) (*yarnpb.Program, error) {
	prog := new(yarnpb.Program)
	if err := proto.Unmarshal(yarnc, prog); err != nil {
		return nil, fmt.Errorf("unmarshaling program: %w", err)
	}
	return prog, nil
}
