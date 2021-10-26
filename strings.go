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
	"sort"
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
// (from line.Substitutions), applies format functions, and processes style
// tags into attributes.
func (t *StringTable) Render(line Line) (*AttributedString, error) {
	row, found := t.Table[line.ID]
	if !found {
		return nil, fmt.Errorf("no string %q in string table", line.ID)
	}
	// Parse the line according to the grammar below.
	filename := fmt.Sprintf("%s:%d", row.File, row.LineNumber)
	pl := new(parsedString)
	if err := lineParser.ParseString(filename, row.Text, pl); err != nil {
		return nil, err
	}

	// Apply substitutions and format functions into a string builder.
	var asb attStrBuilder
	if err := pl.render(&asb, line.Substitutions, t.Language); err != nil {
		return nil, err
	}
	return asb.attStr(), nil
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

func (p *parsedString) render(asb *attStrBuilder, substs []string, lang language.Tag) error {
	for _, f := range p.Fragments {
		if err := f.render(asb, substs, lang); err != nil {
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

func (s *fragment) render(asb *attStrBuilder, substs []string, lang language.Tag) error {
	if s == nil {
		return nil
	}
	switch {
	case s.Escaped != "":
		asb.WriteString(s.Escaped[1:])
	case s.Markup != nil:
		return s.Markup.render(asb, substs, lang)
	case s.Subst != "":
		n, err := strconv.Atoi(s.Subst)
		if err != nil || n < 0 || n >= len(substs) {
			asb.WriteString("{" + s.Subst + "}")
			break
		}
		asb.WriteString(substs[n])
	default:
		asb.WriteString(s.Text)
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

func (f *parsedMarkup) render(asb *attStrBuilder, substs []string, lang language.Tag) error {
	// input is a fragment that needs assembling
	var in string
	if f.Input != nil {
		var inb attStrBuilder
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
		return f.findAndRender(asb, substs, in, in, lang)

	case "plural":
		ops, err := cldr.NewOperands(in)
		if err != nil {
			return err
		}
		form := plural.Cardinal.MatchPlural(lang, int(ops.I), int(ops.V), int(ops.W), int(ops.F), int(ops.T))
		if int(form) > len(formKeyTable) {
			return fmt.Errorf("plural form %v not supported", form)
		}
		return f.findAndRender(asb, substs, in, formKeyTable[form], lang)

	case "ordinal":
		ops, err := cldr.NewOperands(in)
		if err != nil {
			return err
		}
		form := plural.Ordinal.MatchPlural(lang, int(ops.I), int(ops.V), int(ops.W), int(ops.F), int(ops.T))
		if int(form) > len(formKeyTable) {
			return fmt.Errorf("plural form %v not supported", form)
		}
		return f.findAndRender(asb, substs, in, formKeyTable[form], lang)

	default:
		// Something else. Style tag I hope.
		switch {
		case f.OpeningSlash == "/" && f.Name == "":
			// Close-all tag
			asb.closeAll()

		case f.OpeningSlash == "/":
			// Close tag
			if err := asb.closeTag(f.Name); err != nil {
				return err
			}

		case f.ClosingSlash == "/":
			// Self-closing tag
			if err := asb.openTag(f.Name, f.Props, substs, lang); err != nil {
				return err
			}
			if err := asb.closeTag(f.Name); err != nil {
				return err
			}

		default:
			// Open tag
			if err := asb.openTag(f.Name, f.Props, substs, lang); err != nil {
				return err
			}
		}
		return nil
	}
}

// findAndRender searches f.Props for the option matching the key, and then
// renders that option to sb.
func (f *parsedMarkup) findAndRender(asb *attStrBuilder, substs []string, input, key string, lang language.Tag) error {
	for _, opt := range f.Props {
		if opt.Key == key {
			return opt.render(asb, substs, input, lang)
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

func (p *parsedProp) render(asb *attStrBuilder, substs []string, input string, lang language.Tag) error {
	// Property values have an additional token that needs to be processed
	// specially (%).
	for _, v := range p.Value.Fragments {
		if v.Text == "%" {
			asb.WriteString(input)
			continue
		}
		if err := v.render(asb, substs, lang); err != nil {
			return err
		}
	}
	return nil
}

// AttributedString is a string with additional attributes, such as presentation
// or styling information, that apply to the whole string or substrings.
type AttributedString struct {
	Str        string
	Attributes []*Attribute
}

func (s *AttributedString) String() string { return s.Str }

// ScanAttribEvents calls visit with each Attribute twice: once at the start of
// the attribute, and once at the end. The visit calls will happen in order, but
// not necessarily the same order as they were originally presented in the
// source string.
//
// For example, an attributed string built from the input
// `[a]Here's A![/a][b]Here's B! [c]With C now[/c][/b]`
// could be visited in the order
// (`a`, start), (`b`, start), (`a`, end), (`c`, start), (`b`, end), (`c`, end).
func (s *AttributedString) ScanAttribEvents(visit func(att *Attribute, start bool)) {
	// Make a reference
	starts := s.Attributes
	// Sort starts by start
	sort.Slice(starts, func(i, j int) bool {
		return starts[i].Start < starts[j].Start
	})
	// Make a *copy*
	ends := append([]*Attribute(nil), s.Attributes...)
	// Sort ends by end
	sort.Slice(ends, func(i, j int) bool {
		return ends[i].End < ends[j].End
	})
	// March through starts and ends, and do visiting.
	// If the attributed string is well-formed (i.e. start <= end for every
	// attribute), all the starts must be visited before all the ends, so really
	// the loop only has to check i < len(starts).
	// But check j < len(ends) anwyay to avoid a panic in case it is malformed.
	i, j := 0, 0
	for i < len(starts) && j < len(ends) {
		if starts[i].Start <= ends[j].End {
			visit(starts[i], true)
			i++
		} else {
			visit(ends[j], false)
			j++
		}
	}
	for ; j < len(ends); j++ {
		visit(ends[j], false)
	}
}

// Attribute describes a range within a string with additional information
// provided by markup tags. Start and End specify the range in bytes. Name is
// the tag name, and Props contains any additional key="value" tag properties.
type Attribute struct {
	Start, End int
	Name       string
	Props      map[string]string
}

type attStrBuilder struct {
	strings.Builder
	attribs []*Attribute
	open    map[string][]*Attribute // lazily created (by openTag)
}

func (b *attStrBuilder) attStr() *AttributedString {
	return &AttributedString{
		Str:        b.Builder.String(),
		Attributes: b.attribs,
	}
}

func (b *attStrBuilder) openTag(name string, props []*parsedProp, substs []string, lang language.Tag) error {
	// Render each prop value into its own string, and put into a map
	m := make(map[string]string)
	for _, prop := range props {
		var vsb attStrBuilder
		if err := prop.Value.render(&vsb, substs, lang); err != nil {
			return err
		}
		// So ... attributed strings *could* have attributes that have
		// properties that have values that are attributed strings ...
		// Haha lol nope.
		m[prop.Key] = vsb.String()
	}
	a := &Attribute{
		Start: b.Builder.Len(),
		Name:  name,
		Props: m,
	}
	if b.open == nil {
		b.open = map[string][]*Attribute{name: {a}}
		return nil
	}
	b.open[name] = append(b.open[name], a)
	return nil
}

func (b *attStrBuilder) closeTag(name string) error {
	if b.open == nil {
		return fmt.Errorf("tag %q not open", name)
	}
	as := b.open[name]
	l := len(as)
	if l == 0 {
		return fmt.Errorf("tag %q not open", name)
	}
	// Close the last one
	a, as := as[l-1], as[:l-1]
	b.open[name] = as
	a.End = b.Builder.Len()
	b.attribs = append(b.attribs, a)
	return nil
}

func (b *attStrBuilder) closeAll() {
	for name, as := range b.open {
		for _, a := range as {
			a.End = b.Builder.Len()
			b.attribs = append(b.attribs, a)
		}
		delete(b.open, name)
	}
}
