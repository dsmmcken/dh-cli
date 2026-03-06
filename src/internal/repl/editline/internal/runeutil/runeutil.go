// Package runeutil provides utility functions for tidying up incoming runes
// from Key messages. Inlined from charm.land/bubbles/v2/internal/runeutil.
package runeutil

import (
	"unicode"
	"unicode/utf8"
)

// Sanitizer is a helper for bubble widgets that want to process
// Runes from input key messages.
type Sanitizer interface {
	Sanitize(runes []rune) []rune
}

// NewSanitizer constructs a rune sanitizer.
func NewSanitizer(opts ...Option) Sanitizer {
	s := sanitizer{
		replaceNewLine: []rune("\n"),
		replaceTab:     []rune("    "),
	}
	for _, o := range opts {
		s = o(s)
	}
	return &s
}

// Option is the type of option that can be passed to NewSanitizer.
type Option func(sanitizer) sanitizer

// ReplaceTabs replaces tabs by the specified string.
func ReplaceTabs(tabRepl string) Option {
	return func(s sanitizer) sanitizer {
		s.replaceTab = []rune(tabRepl)
		return s
	}
}

// ReplaceNewlines replaces newline characters by the specified string.
func ReplaceNewlines(nlRepl string) Option {
	return func(s sanitizer) sanitizer {
		s.replaceNewLine = []rune(nlRepl)
		return s
	}
}

func (s *sanitizer) Sanitize(runes []rune) []rune {
	dstrunes := runes[:0:len(runes)]
	copied := false

	for src := range runes {
		r := runes[src]
		switch {
		case r == utf8.RuneError:
			// skip

		case r == '\r' || r == '\n':
			if len(dstrunes)+len(s.replaceNewLine) > src && !copied {
				dst := len(dstrunes)
				dstrunes = make([]rune, dst, len(runes)+len(s.replaceNewLine))
				copy(dstrunes, runes[:dst])
				copied = true
			}
			dstrunes = append(dstrunes, s.replaceNewLine...)

		case r == '\t':
			if len(dstrunes)+len(s.replaceTab) > src && !copied {
				dst := len(dstrunes)
				dstrunes = make([]rune, dst, len(runes)+len(s.replaceTab))
				copy(dstrunes, runes[:dst])
				copied = true
			}
			dstrunes = append(dstrunes, s.replaceTab...)

		case unicode.IsControl(r):
			// Other control characters: skip.

		default:
			dstrunes = append(dstrunes, runes[src])
		}
	}
	return dstrunes
}

type sanitizer struct {
	replaceNewLine []rune
	replaceTab     []rune
}
