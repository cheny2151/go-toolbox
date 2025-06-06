package stringtool

import (
	"fmt"
	"testing"
)

func TestTokenParser(t *testing.T) {
	sql := "select * from a where a = #{test} and b = #{test2} and c = 1"
	parser := TokenParser(sql, "#{", "}", func(key string) string {
		return "@" + key
	})
	fmt.Println(parser)
}
