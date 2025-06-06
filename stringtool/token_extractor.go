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
out:
	for cursor := 0; cursor < len(text); {
		if chars[cursor] == start[0] {
			for i := 0; i < startTokenLen && cursor+i < len(text); {
				if start[i] != chars[cursor+i] {
					break
				}
				i++
				if i == startTokenLen {
					startLabel = cursor + startTokenLen
					cursor++
					continue out
				}
			}
		}
		if chars[cursor] == end[0] {
			for i := 0; i < endTokenLen && cursor+i < len(text); {
				if end[i] != chars[cursor+i] {
					break
				}
				i++
				if i == endTokenLen && startLabel != -1 {
					results = append(results, text[startLabel:cursor])
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

func (extractor *TokenExtractor) find(target []rune, chars []rune, cursor int) int {
	targetLen := len(target)
	idx := 0
	c := target[0]
	for i := cursor; i < len(chars); i++ {
		if c == chars[i] {
			idx++
			if idx == targetLen {
				return i
			}
			c = target[idx]
		} else if idx != 0 {
			idx = 0
			c = target[0]
		}
	}
	return -1
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
	textChars := []rune(text)
	startIdx := -1
	tail := 0
	for cursor := 0; cursor < len(textChars); {
		char := textChars[cursor]
		isSpace := IsSpace(char)

		if startIdx != -1 && ((isSpace && len(endChars) == 0) ||
			(len(endChars) > 0 && matchChars(textChars, endChars, cursor))) {
			builder.WriteString(string(textChars[tail:startIdx]))
			key := string(textChars[startIdx+len(startChars) : cursor])
			replace := apply(key)
			builder.WriteString(replace)
			tail = cursor + len(endChars)
			if cursor == tail {
				cursor++
			} else {
				cursor = tail
			}
			startIdx = -1
			continue
		} else if isSpace {
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
