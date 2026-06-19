package lsp

import (
	"go.lsp.dev/protocol"

	"sova/internal/services/compiler"
)

func cssClassCompletions(c *compiler.CompilerContext, slot *callContext) []protocol.CompletionItem {
	classes := projectCSSClasses(c)
	if len(classes) == 0 {
		return nil
	}

	detail := "CSS class"
	sortPrefix := ""
	if slot != nil {
		detail = "CSS class . " + slot.callee + " (arg #" + itoa(slot.argIndex+1) + ")"
		sortPrefix = "0"
	}

	out := make([]protocol.CompletionItem, 0, len(classes))
	for _, name := range classes {
		item := protocol.CompletionItem{
			Label:  name,
			Detail: detail,
			Kind:   protocol.CompletionItemKindColor,
		}

		if sortPrefix != "" {
			item.SortText = sortPrefix + name
		}

		out = append(out, item)
	}

	return out
}

func classNameAtCursor(src string, offset int) (string, int, int, bool) {
	if _, ok := openingQuoteOfEnclosingString(src, offset); !ok {
		return "", 0, 0, false
	}

	start := offset
	for start > 0 && isClassIdentByte(src[start-1]) {
		start--
	}

	end := offset
	for end < len(src) && isClassIdentByte(src[end]) {
		end++
	}

	if start == end {
		return "", 0, 0, false
	}

	name := src[start:end]
	if name == "" || !isClassNameValid(name) {
		return "", 0, 0, false
	}

	return name, start, end, true
}

func isClassIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

func isClassNameValid(name string) bool {
	if name == "" {
		return false
	}

	i := 0
	if name[0] == '-' {
		i = 1
	}

	if i >= len(name) {
		return false
	}

	first := name[i]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}

	return true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	neg := false
	if n < 0 {
		neg = true
		n = -n
	}

	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}

	if neg {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}
