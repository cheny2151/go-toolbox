package stringtool

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestTokenExtractor_Extract(t *testing.T) {
	extractor := CreateTokenExtractor("${", "}")

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "基本单个token",
			input: "${foo}",
			want:  []string{"foo"},
		},
		{
			name:  "多个token",
			input: "${foo} and ${bar}",
			want:  []string{"foo", "bar"},
		},
		{
			name:  "token前后有文本",
			input: "hello ${name}!",
			want:  []string{"name"},
		},
		{
			name:  "空字符串",
			input: "",
			want:  []string{},
		},
		{
			name:  "无token",
			input: "hello world",
			want:  []string{},
		},
		{
			name:  "空内容token",
			input: "${}",
			want:  []string{""},
		},
		{
			name:  "未闭合token",
			input: "${foo",
			want:  []string{},
		},
		{
			name:  "end在start之前",
			input: "}foo{",
			want:  []string{},
		},
		{
			name:  "相邻token",
			input: "${a}${b}",
			want:  []string{"a", "b"},
		},
		{
			name:  "三个token",
			input: "${foo}${bar}${baz}",
			want:  []string{"foo", "bar", "baz"},
		},
		{
			name:  "token前有单个$符号",
			input: "$${foo}",
			want:  []string{"foo"},
		},
		{
			name:  "只有$符号无token",
			input: "$foo",
			want:  []string{},
		},
		{
			name:  "token中含空格",
			input: "${foo bar}",
			want:  []string{"foo bar"},
		},
		{
			name:  "token中含换行",
			input: "${fo\no}",
			want:  []string{"fo\no"},
		},
		{
			name:  "token中含$符号",
			input: "${foo$bar}",
			want:  []string{"foo$bar"},
		},
		{
			name:  "token中含{符号",
			input: "${foo{bar}",
			want:  []string{"foo{bar"},
		},
		{
			name:  "嵌套start token覆盖startLabel",
			input: "${${foo}}",
			// 第二个 ${ 会覆盖 startLabel，提取 "foo"，尾部 } 被忽略
			want: []string{"foo"},
		},
		{
			name:  "仅end符号",
			input: "}",
			want:  []string{},
		},
		{
			name:  "多个连续end在start之后",
			input: "${foo}}",
			want:  []string{"foo"},
		},
		{
			name:  "多行文本多个token",
			input: "line1\n${a}\nline2\n${b}\nline3",
			want:  []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractor.Extract(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Extract(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestTokenExtractor_Extract_Unicode 验证多字节Unicode字符场景
// 当前实现存在Bug：cursor < len(text) 使用字节长度而非rune数量，
// 且 text[startLabel:cursor] 混用了rune下标和字节下标，会导致panic或结果错误。
func TestTokenExtractor_Extract_Unicode(t *testing.T) {
	extractor := CreateTokenExtractor("${", "}")

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "token前有中文",
			input: "你好${name}",
			want:  []string{"name"},
		},
		{
			name:  "token后有中文",
			input: "${name}世界",
			want:  []string{"name"},
		},
		{
			name:  "token内容为中文",
			input: "${名前}",
			want:  []string{"名前"},
		},
		{
			name:  "中文环绕多个token",
			input: "你好${foo}世界${bar}！",
			want:  []string{"foo", "bar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 当前实现对多字节Unicode会panic，使用defer recover捕获
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Extract(%q) panicked: %v (存在Unicode Bug)", tt.input, r)
				}
			}()
			got := extractor.Extract(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Extract(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCreateTokenExtractor_Panic(t *testing.T) {
	tests := []struct {
		name  string
		start string
		end   string
	}{
		{"空start", "", "}"},
		{"空end", "${", ""},
		{"两者均空", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("CreateTokenExtractor(%q, %q) 应该panic但未panic", tt.start, tt.end)
				}
			}()
			CreateTokenExtractor(tt.start, tt.end)
		})
	}
}

// upper 用作测试 apply 函数，将 key 转为大写并包裹括号
func upper(key string) string {
	return "[" + strings.ToUpper(key) + "]"
}

// identity 原样返回 key
func identity(key string) string {
	return key
}

func TestTokenParser_WithEnd(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		start string
		end   string
		apply func(string) string
		want  string
	}{
		{
			name:  "基本单个token",
			text:  "${name}",
			start: "${", end: "}",
			apply: upper,
			want:  "[NAME]",
		},
		{
			name:  "token嵌在文本中",
			text:  "hello ${name}!",
			start: "${", end: "}",
			apply: upper,
			want:  "hello [NAME]!",
		},
		{
			name:  "多个token",
			text:  "${foo} and ${bar}",
			start: "${", end: "}",
			apply: upper,
			want:  "[FOO] and [BAR]",
		},
		{
			name:  "相邻token无间隔",
			text:  "${a}${b}",
			start: "${", end: "}",
			apply: upper,
			want:  "[A][B]",
		},
		{
			name:  "空内容token",
			text:  "${}",
			start: "${", end: "}",
			apply: func(key string) string { return "(empty)" },
			want:  "(empty)",
		},
		{
			name:  "apply返回空字符串",
			text:  "hello ${name} world",
			start: "${", end: "}",
			apply: func(key string) string { return "" },
			want:  "hello  world",
		},
		{
			name:  "apply返回含start token的字符串(不触发二次解析)",
			text:  "${key}",
			start: "${", end: "}",
			apply: func(key string) string { return "${not-replaced}" },
			want:  "${not-replaced}",
		},
		{
			name:  "未闭合token原样输出",
			text:  "hello ${foo",
			start: "${", end: "}",
			apply: upper,
			want:  "hello ${foo",
		},
		{
			name:  "end在start之前原样输出",
			text:  "}foo${",
			start: "${", end: "}",
			apply: upper,
			want:  "}foo${",
		},
		{
			name:  "无token原样输出",
			text:  "hello world",
			start: "${", end: "}",
			apply: upper,
			want:  "hello world",
		},
		{
			name:  "空字符串",
			text:  "",
			start: "${", end: "}",
			apply: upper,
			want:  "",
		},
		{
			name:  "start为空直接返回原文",
			text:  "${foo}",
			start: "", end: "}",
			apply: upper,
			want:  "${foo}",
		},
		{
			name:  "嵌套start token:内层覆盖外层startIdx",
			text:  "${outer${inner}suffix",
			start: "${", end: "}",
			apply: upper,
			// 内层 ${ 覆盖 startIdx，outer 前的内容 "${outer" 原样写出，
			// 提取 "inner"，"suffix" 无 end 原样写出
			want: "${outer[INNER]suffix",
		},
		{
			name:  "token中含Unicode字符",
			text:  "你好 ${名前}！",
			start: "${", end: "}",
			apply: func(key string) string { return "world" },
			want:  "你好 world！",
		},
		{
			name:  "多行文本",
			text:  "line1\n${a}\nline2\n${b}\nline3",
			start: "${", end: "}",
			apply: upper,
			want:  "line1\n[A]\nline2\n[B]\nline3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TokenParser(tt.text, tt.start, tt.end, tt.apply)
			if got != tt.want {
				t.Errorf("TokenParser(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

// TestTokenParser_EmptyEnd 验证 end="" 时以空白符为终止符的场景（类似 @mention 风格）
func TestTokenParser_EmptyEnd(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		start string
		apply func(string) string
		want  string
	}{
		{
			name:  "token后跟空格",
			text:  "@name rest",
			start: "@",
			apply: upper,
			want:  "[NAME] rest",
		},
		{
			name:  "多个token均有尾随空格",
			text:  "@first @last end",
			start: "@",
			apply: upper,
			want:  "[FIRST] [LAST] end",
		},
		{
			name:  "token前有文本",
			text:  "hello @name world",
			start: "@",
			apply: upper,
			want:  "hello [NAME] world",
		},
		{
			name:  "无token",
			text:  "hello world",
			start: "@",
			apply: upper,
			want:  "hello world",
		},
		{
			// Bug: end="" 且 token 位于字符串末尾（无尾随空格），apply 不会被调用，原样输出
			name:  "【Bug】token位于字符串末尾无尾随空格",
			text:  "hello @name",
			start: "@",
			apply: upper,
			want:  "hello [NAME]", // 期望值，当前实现返回 "hello @name"
		},
		{
			// Bug: 整个字符串只有一个 token 且无尾随空格
			name:  "【Bug】字符串仅为单个token",
			text:  "@name",
			start: "@",
			apply: upper,
			want:  "[NAME]", // 期望值，当前实现返回 "@name"
		},
		{
			// Bug: 最后一个 token 无尾随空格
			name:  "【Bug】多token最后一个无尾随空格",
			text:  "@first @last",
			start: "@",
			apply: upper,
			want:  "[FIRST] [LAST]", // 期望值，当前实现返回 "[FIRST] @last"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TokenParser(tt.text, tt.start, "", tt.apply)
			if got != tt.want {
				t.Errorf("TokenParser(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestTokenParser(t *testing.T) {
	sql := "select * from a where a = #{test} and b = #{test2} and c = @name "
	parser := TokenParser(sql, "@", "", func(key string) string {
		return key + "_value"
	})
	fmt.Println(string(parser[len(parser)-1]))
}
