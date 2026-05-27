package stringtool

import (
	"strings"
	"unicode"
)

type TokenExtractor struct {
	start []rune
	end   []rune
}

func CreateTokenExtractor(start string, end string) *TokenExtractor {
	if len(start) == 0 || len(end) == 0 {
		panic("start or end token can not be empty")
	}
	return &TokenExtractor{
		start: []rune(start),
		end:   []rune(end),
	}
}

func (extractor *TokenExtractor) Extract(text string) []string {
	start := extractor.start
	end := extractor.end
	results := make([]string, 0)
	chars := []rune(text)
	startTokenLen := len(start)
	endTokenLen := len(end)
	startLabel := -1
	charsLen := len(chars)
out:
	for cursor := 0; cursor < charsLen; {
		if chars[cursor] == start[0] {
			for i := 0; i < startTokenLen && cursor+i < charsLen; {
				if start[i] != chars[cursor+i] {
					break
				}
				i++
				if i == startTokenLen {
					startLabel = cursor + startTokenLen
					cursor += startTokenLen
					continue out
				}
			}
		}
		if chars[cursor] == end[0] {
			for i := 0; i < endTokenLen && cursor+i < charsLen; {
				if end[i] != chars[cursor+i] {
					break
				}
				i++
				if i == endTokenLen && startLabel != -1 {
					results = append(results, string(chars[startLabel:cursor]))
					startLabel = -1
					cursor += i
					continue out
				}
			}
		}
		cursor++
	}
	return results
}

// TokenParser 匹配start和end，并执行替换函数；中间的key执行apply进行替换
// start不可为空,end可以为空;e.g.: @test
func TokenParser(text, start, end string, apply func(key string) string) string {
	if text == "" || start == "" {
		return text
	}
	builder := strings.Builder{}
	startChars := []rune(start)
	endChars := []rune(end)
	hasEndChars := len(endChars) > 0
	textChars := []rune(text)
	textCharsLen := len(textChars)
	startIdx := -1
	tail := 0
	for cursor := 0; cursor < textCharsLen; {
		char := textChars[cursor]
		isPace := IsSpace(char)

		if startIdx != -1 {
			var key *string
			var endWhenFinished bool
			if hasEndChars && matchChars(textChars, endChars, cursor) {
				s := string(textChars[startIdx+len(startChars) : cursor])
				key = &s
			}
			if !hasEndChars {
				if isPace {
					s := string(textChars[startIdx+len(startChars) : cursor])
					key = &s
				} else if cursor == textCharsLen-1 {
					endWhenFinished = true
					s := string(textChars[startIdx+len(startChars) : textCharsLen])
					key = &s
				}
			}
			if key != nil {
				builder.WriteString(string(textChars[tail:startIdx]))
				replace := apply(*key)
				builder.WriteString(replace)

				if endWhenFinished {
					cursor, tail = textCharsLen, textCharsLen
				} else {
					tail = cursor + len(endChars)
					if !hasEndChars {
						cursor++
					} else {
						cursor = tail
					}
					startIdx = -1
				}
				continue
			}
		}

		if isPace {
			startIdx = -1
		} else if matchChars(textChars, startChars, cursor) {
			startIdx = cursor
		}
		cursor++
	}
	if tail < len(textChars) {
		builder.WriteString(string(textChars[tail:]))
	}
	return builder.String()
}

func IsSpace(char rune) bool {
	return unicode.IsSpace(char)
}

func matchChars(values, target []rune, idx int) bool {
	for i := 0; i < len(target); i++ {
		cursor := idx + i
		if cursor >= len(values) || values[cursor] != target[i] {
			return false
		}
	}
	return true
}
