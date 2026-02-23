package response

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// jsonTokenType identifies the kind of JSON token for syntax coloring.
type jsonTokenType int

const (
	jsonTokenKey jsonTokenType = iota
	jsonTokenString
	jsonTokenNumber
	jsonTokenBool
	jsonTokenNull
	jsonTokenPunct
	jsonTokenWhitespace
)

// jsonToken holds a single lexed JSON token.
type jsonToken struct {
	typ   jsonTokenType
	value string
}

// tokenColorName maps token types to Fyne theme color names.
var tokenColorName = map[jsonTokenType]fyne.ThemeColorName{
	jsonTokenKey:        theme.ColorNamePrimary,
	jsonTokenString:     theme.ColorNameSuccess,
	jsonTokenNumber:     theme.ColorNameWarning,
	jsonTokenBool:       theme.ColorNameError,
	jsonTokenNull:       theme.ColorNameDisabled,
	jsonTokenPunct:      theme.ColorNameForeground,
	jsonTokenWhitespace: theme.ColorNameForeground,
}

// highlightJSON converts a pretty-printed JSON string into colored RichText segments.
func highlightJSON(input string) []widget.RichTextSegment {
	if input == "" {
		return nil
	}

	tokens := tokenizeJSON(input)
	segments := make([]widget.RichTextSegment, 0, len(tokens))

	for _, tok := range tokens {
		colorName := tokenColorName[tok.typ]
		segments = append(segments, &widget.TextSegment{
			Style: widget.RichTextStyle{
				ColorName: colorName,
				Inline:    true,
				SizeName:  theme.SizeNameText,
				TextStyle: fyne.TextStyle{Monospace: true},
			},
			Text: tok.value,
		})
	}

	return segments
}

// tokenizeJSON breaks a JSON string into typed tokens.
func tokenizeJSON(input string) []jsonToken {
	tokens := make([]jsonToken, 0, 128)
	i := 0

	for i < len(input) {
		ch := input[i]

		switch {
		case ch == '"':
			j := i + 1
			for j < len(input) {
				if input[j] == '\\' {
					j += 2
					continue
				}
				if input[j] == '"' {
					j++
					break
				}
				j++
			}
			tokens = append(tokens, jsonToken{typ: jsonTokenString, value: input[i:j]})
			i = j

		case ch == '-' || (ch >= '0' && ch <= '9'):
			j := i + 1
			for j < len(input) {
				c := input[j]
				if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
					j++
				} else {
					break
				}
			}
			tokens = append(tokens, jsonToken{typ: jsonTokenNumber, value: input[i:j]})
			i = j

		case ch == 't' && i+4 <= len(input) && input[i:i+4] == "true":
			tokens = append(tokens, jsonToken{typ: jsonTokenBool, value: "true"})
			i += 4

		case ch == 'f' && i+5 <= len(input) && input[i:i+5] == "false":
			tokens = append(tokens, jsonToken{typ: jsonTokenBool, value: "false"})
			i += 5

		case ch == 'n' && i+4 <= len(input) && input[i:i+4] == "null":
			tokens = append(tokens, jsonToken{typ: jsonTokenNull, value: "null"})
			i += 4

		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			j := i + 1
			for j < len(input) {
				c := input[j]
				if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
					j++
				} else {
					break
				}
			}
			tokens = append(tokens, jsonToken{typ: jsonTokenWhitespace, value: input[i:j]})
			i = j

		case ch == '{' || ch == '}' || ch == '[' || ch == ']' || ch == ':' || ch == ',':
			tokens = append(tokens, jsonToken{typ: jsonTokenPunct, value: string(ch)})
			i++

		default:
			// Unexpected character — emit as punctuation
			tokens = append(tokens, jsonToken{typ: jsonTokenPunct, value: string(ch)})
			i++
		}
	}

	// Second pass: promote strings that precede a colon to keys.
	for idx := 0; idx < len(tokens); idx++ {
		if tokens[idx].typ == jsonTokenString {
			for j := idx + 1; j < len(tokens); j++ {
				if tokens[j].typ == jsonTokenWhitespace {
					continue
				}
				if tokens[j].typ == jsonTokenPunct && tokens[j].value == ":" {
					tokens[idx].typ = jsonTokenKey
				}
				break
			}
		}
	}

	return tokens
}
