package tool

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
