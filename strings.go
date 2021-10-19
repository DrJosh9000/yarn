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
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"github.com/alecthomas/participle"
)

// StringTableRow contains all the information from one row in a string table.
type StringTableRow struct {
	ID, Text, File, Node string
	LineNumber           int
}

// StringTable contains all the information from a string table, keyed by
// string ID.
type StringTable struct {
	langCode string
	table    map[string]StringTableRow
}

// ReadStringTable reads a CSV-formatted string table from the reader. It
// assumes the first row is a header.
func ReadStringTable(r io.Reader, langCode string) (*StringTable, error) {
	st := make(map[string]StringTableRow)
	header := true
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = 5
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("csv read: %v", err)
		}
		if header {
			header = false
			continue
		}
		ln, err := strconv.Atoi(rec[4])
		if err != nil {
			return nil, fmt.Errorf("atoi: %w", err)
		}
		id := rec[0]
		st[id] = StringTableRow{
			ID:         id,
			Text:       rec[1],
			File:       rec[2],
			Node:       rec[3],
			LineNumber: ln,
		}
	}
	return &StringTable{
		langCode: langCode,
		table:    st,
	}, nil
}

// Render looks up the row corresponding to line.ID, interpolates substitutions
// (from line.Substitutions), and applies format functions.
func (t *StringTable) Render(line Line) (string, error) {
	row, found := t.table[line.ID]
	if !found {
		return "", fmt.Errorf("no string %q in string table", line.ID)
	}
	return t.render(row.Text, line.Substitutions)
}

func (t *StringTable) render(text string, substs []string) (string, error) {
	// Do substitutions and format functions in one stateful loop.
	var sb strings.Builder
	const (
		raw = iota
		subst
		fmtf
		fmtfstr
	)
	escape := false // true means next rune is escaped
	stack, state := []int{}, raw
	push := func(st int) {
		stack, state = append(stack, state), st
	}
	pop := func() {
		stack, state = stack[:len(stack)-1], stack[len(stack)-1]
	}
	substIdx := 0
	var ffstart int

	for i, r := range text {
		switch state {
		case raw:
			if escape {
				switch r {
				case '[', ']', '{', '}':
					// escape sequences in raw state are \[, \], \{, and \}
				default:
					// this char is not part of an escape sequence
					sb.WriteRune('\\')
				}
				sb.WriteRune(r)
				escape = false
				break
			}
			switch r {
			case '\\':
				escape = true
			case '[':
				ffstart = i
				push(fmtf)
			case '{':
				push(subst)
			default:
				sb.WriteRune(r)
			}

		case subst:
			switch {
			case unicode.IsDigit(r):
				substIdx *= 10
				substIdx += int(r - '0')
			case r == '}':
				if substIdx < 0 || substIdx >= len(substs) {
					return "", fmt.Errorf("substitution index %d out of range [0, %d)", substIdx, len(substs))
				}
				sb.WriteString(substs[substIdx])
				pop()
			default:
				return "", fmt.Errorf("invalid rune %c in substitution", r)
			}

		case fmtf:
			// find the range of the format function within the string
			if escape {
				escape = false
				break
			}
			switch r {
			case '\\':
				escape = true
			case '"':
				// a string within the format function
				push(fmtfstr)
			case ']':
				// parse and evaluate the function
				ff := new(fmtFunc)
				if err := funcParser.ParseString(text[ffstart+1:i], ff); err != nil {
					return "", err
				}
				fr, err := ff.render(substs, t.langCode)
				if err != nil {
					return "", err
				}
				sb.WriteString(fr)
				pop()
			}

		case fmtfstr:
			// a string within function
			if escape {
				escape = false
				break
			}
			switch r {
			case '\\':
				escape = true
			case '"':
				// end of string
				pop()
			}
		}
	}
	if escape {
		// trailing backslash is not an escape sequence
		sb.WriteRune('\\')
	}
	return sb.String(), nil
}

type fmtFunc struct {
	Name  string `parser:"@Ident"`
	Input string `parser:"@String"`
	Opts  []struct {
		K string `parser:"@Ident '='"`
		V string `parser:"@String"`
	} `parser:"@@+"`
}

var funcParser = participle.MustBuild(&fmtFunc{})

func (f *fmtFunc) render(substs []string, langCode string) (string, error) {
	// is the input a substitution?
	if strings.HasPrefix(f.Input, "{") && strings.HasSuffix(f.Input, "}") {
		n, err := strconv.Atoi(strings.Trim(f.Input, "{}"))
		if err != nil {
			return "", fmt.Errorf("invalid substitution: %w", err)
		}
		if n < 0 || n >= len(substs) {
			return "", fmt.Errorf("substitution index %d out of range [0, %d)", n, len(substs))
		}
		f.Input = substs[n]
	}

	switch f.Name {
	case "select":
		// input chooses which value to interpolate directly
		for _, kv := range f.Opts {
			if kv.K == f.Input {
				return kv.V, nil
			}
		}
		return "", fmt.Errorf("key %q not found in %v", f.Input, f.Opts)

	case "plural":
		// input is itself a number to feed through CLDR to find the
		// pluralisation case
		// TODO

	case "ordinal":
		// input is itself a number to feed through CLDR to find the
		// ordinal case
		// TODO

	}
	return "", fmt.Errorf("unknown format function %q", f.Name)

}
