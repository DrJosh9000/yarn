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
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
	cldr "github.com/razor-1/localizer-cldr"
	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

// StringTableRow contains all the information from one row in a string table.
type StringTableRow struct {
	ID, Text, File, Node string
	LineNumber           int
}

// StringTable contains all the information from a string table, keyed by
// string ID. This can be constructed either by using ReadStringTable, or
// manually (e.g. if you are not using Yarn Spinner CSV string tables).
type StringTable struct {
	Language language.Tag
	Table    map[string]StringTableRow
}

// LoadStringTableFile is a convenient function for loading a CSV string table
// given a file path. It assumes the first row is a header. langCode must be a
// valid BCP 47 language tag.
func LoadStringTableFile(stringTablePath, langCode string) (*StringTable, error) {
	csv, err := os.Open(stringTablePath)
	if err != nil {
		return nil, fmt.Errorf("opening string table file: %w", err)
	}
	defer csv.Close()
	st, err := ReadStringTable(csv, langCode)
	if err != nil {
		return nil, fmt.Errorf("reading string table: %w", err)
	}
	return st, nil
}

// ReadStringTable reads a CSV string table from the reader. It assumes the
// first row is a header. langCode must be a valid BCP 47 language tag.
func ReadStringTable(r io.Reader, langCode string) (*StringTable, error) {
	lang, err := language.Parse(langCode)
	if err != nil {
		return nil, fmt.Errorf("invalid lang code: %w", err)
	}

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
		Language: lang,
		Table:    st,
	}, nil
}

// Render looks up the row corresponding to line.ID, interpolates substitutions
// (from line.Substitutions), and applies format functions.
func (t *StringTable) Render(line Line) (string, error) {
	row, found := t.Table[line.ID]
	if !found {
		return "", fmt.Errorf("no string %q in string table", line.ID)
	}
	// Parse the line according to the grammar below.
	filename := fmt.Sprintf("%s:%d", row.File, row.LineNumber)
	pl := new(parsedString)
	if err := lineParser.ParseString(filename, row.Text, pl); err != nil {
		return "", err
	}

	// Apply substitutions and format functions into a string builder.
	var sb strings.Builder
	if err := pl.render(&sb, line.Substitutions, t.Language); err != nil {
		return "", err
	}
	return sb.String(), nil
}

var (
	// This lexer is a bit more general than needed since it allows things like
	// nested functions, but... hey I get nested functions for ~free!
	lineLexer = lexer.MustStateful(lexer.Rules{
		"Root": {
			{Name: "Escaped", Pattern: `\\[\{\}\[\]"\\]`, Action: nil},
			{Name: "Markup", Pattern: `\[`, Action: lexer.Push("Markup")},
			{Name: "Subst", Pattern: `{`, Action: lexer.Push("Subst")},
			{Name: "Char", Pattern: `[%\{\["\\]|[^%\{\["\\]+`, Action: nil},
		},
		"Markup": {
			{Name: "Whitespace", Pattern: `\s+`, Action: nil},
			{Name: "Slash", Pattern: `/`, Action: nil},
			{Name: "Ident", Pattern: `\w+`, Action: nil},
			{Name: "Equals", Pattern: `=`, Action: nil},
			{Name: "String", Pattern: `"`, Action: lexer.Push("String")},
			{Name: "MarkupEnd", Pattern: `\]`, Action: lexer.Pop()},
		},
		"Subst": {
			{Name: "Index", Pattern: `\d+`, Action: nil},
			{Name: "SubstEnd", Pattern: `}`, Action: lexer.Pop()},
		},
		"String": {
			{Name: "StringEnd", Pattern: `"`, Action: lexer.Pop()},
			lexer.Include("Root"),
		},
	})

	// a line is a kind of string, just missing the quotes...
	lineParser = participle.MustBuild(
		&parsedString{},
		participle.Lexer(lineLexer),
		participle.Elide("Whitespace"),
	)
)

// parsedString is used for both entire lines and the contents of double-quoted
// strings.
type parsedString struct {
	Fragments []*fragment `parser:"@@*"`
}

func (p *parsedString) render(sb *strings.Builder, substs []string, lang language.Tag) error {
	for _, f := range p.Fragments {
		if err := f.render(sb, substs, lang); err != nil {
			return err
		}
	}
	return nil
}

// fragment is part of a string or line. The parser breaks it into pieces so
// that special pieces (escape sequences, markup, substitutions, and %) can be
// processed in a special way.
type fragment struct {
	Escaped string        `parser:"@Escaped"`
	Markup  *parsedMarkup `parser:"| Markup @@ MarkupEnd"`
	Subst   string        `parser:"| Subst @Index SubstEnd"`
	Text    string        `parser:"| @Char"`
}

func (s *fragment) render(sb *strings.Builder, substs []string, lang language.Tag) error {
	if s == nil {
		return nil
	}
	switch {
	case s.Escaped != "":
		sb.WriteString(s.Escaped[1:])
	case s.Markup != nil:
		return s.Markup.render(sb, substs, lang)
	case s.Subst != "":
		n, err := strconv.Atoi(s.Subst)
		if err != nil || n < 0 || n >= len(substs) {
			sb.WriteString("{" + s.Subst + "}")
			break
		}
		sb.WriteString(substs[n])
	default:
		sb.WriteString(s.Text)
	}
	return nil
}

// parsedMarkup is used for both format functions (select, plural, ordinal) and
// BBCode-esque markup tags ([b]Bold!?[/b]).
type parsedMarkup struct {
	OpeningSlash string        `parser:"@Slash?"`                  // indicates closing tag of a pair
	Name         string        `parser:"@Ident?"`                  // used for all except close-all tag [/]
	Input        *parsedString `parser:"( String @@ StringEnd )?"` // used for format funcs
	Props        []*parsedProp `parser:"@@*"`                      // key="value" properties
	ClosingSlash string        `parser:"@Slash?"`                  // indicates self-closing tag
}

// maps plural.Form values to identifiers used in Yarn Spinner plural and
// ordinal format functions
var formKeyTable = []string{
	plural.Other: "other",
	plural.Zero:  "zero",
	plural.One:   "one",
	plural.Two:   "two",
	plural.Few:   "few",
	plural.Many:  "many",
}

func (f *parsedMarkup) render(sb *strings.Builder, substs []string, lang language.Tag) error {
	// input is a fragment that needs assembling
	var in string
	if f.Input != nil {
		var inb strings.Builder
		if err := f.Input.render(&inb, substs, lang); err != nil {
			return err
		}
		in = inb.String()
	}

	// function name determines lookup key
	switch f.Name {
	case "select":
		// input chooses which value to interpolate
		// (input == lookup key)
		return f.findAndRender(sb, substs, in, in, lang)

	case "plural":
		ops, err := cldr.NewOperands(in)
		if err != nil {
			return err
		}
		form := plural.Cardinal.MatchPlural(lang, int(ops.I), int(ops.V), int(ops.W), int(ops.F), int(ops.T))
		if int(form) > len(formKeyTable) {
			return fmt.Errorf("plural form %v not supported", form)
		}
		return f.findAndRender(sb, substs, in, formKeyTable[form], lang)

	case "ordinal":
		ops, err := cldr.NewOperands(in)
		if err != nil {
			return err
		}
		form := plural.Ordinal.MatchPlural(lang, int(ops.I), int(ops.V), int(ops.W), int(ops.F), int(ops.T))
		if int(form) > len(formKeyTable) {
			return fmt.Errorf("plural form %v not supported", form)
		}
		return f.findAndRender(sb, substs, in, formKeyTable[form], lang)

	default:
		// Something else - remove the markup tag from the output for now.
		// TODO: Implement attributed strings
		return nil
	}
}

// findAndRender searches f.Props for the option matching the key, and then
// renders that option to sb.
func (f *parsedMarkup) findAndRender(sb *strings.Builder, substs []string, input, key string, lang language.Tag) error {
	for _, opt := range f.Props {
		if opt.Key == key {
			return opt.render(sb, substs, input, lang)
		}
	}
	return fmt.Errorf("key %q not found in %#v", key, f.Props)
}

// parsedProp is used for key="value" properties of format funcs and markup
// tags.
type parsedProp struct {
	Key   string        `parser:"@Ident Equals"`
	Value *parsedString `parser:"String @@ StringEnd"`
}

func (p *parsedProp) render(sb *strings.Builder, substs []string, input string, lang language.Tag) error {
	// Property values have an additional token that needs to be processed
	// specially (%).
	for _, v := range p.Value.Fragments {
		if v.Text == "%" {
			sb.WriteString(input)
			continue
		}
		if err := v.render(sb, substs, lang); err != nil {
			return err
		}
	}
	return nil
}
