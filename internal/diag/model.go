package diag

import (
	"fmt"
)

type TextSpan struct {
	File     string
	StartLn  int
	StartCol int
	EndLn    int
	EndCol   int
}

func (s TextSpan) String() string {
	if s.File == "" {
		return fmt.Sprintf("(%d:%d-%d:%d)", s.StartLn, s.StartCol, s.EndLn, s.EndCol)
	}

	return fmt.Sprintf("%s:(%d:%d-%d:%d)", s.File, s.StartLn, s.StartCol, s.EndLn, s.EndCol)
}

type DiagnosticLevel int

func (l DiagnosticLevel) String() string {
	switch l {
	case LevelError:
		return "ERR"
	case LevelWarning:
		return "WRN"
	case LevelInfo:
		return "INF"
	default:
		return "UNK"
	}
}

const (
	LevelError DiagnosticLevel = iota

	LevelWarning

	LevelInfo
)

type DiagnosticCategory int

func (c DiagnosticCategory) String() string {
	switch c {
	case CategorySyntax:
		return "SYN"
	case CategorySemantic:
		return "SEM"
	case CategoryType:
		return "TYP"
	case CategoryEmit:
		return "EMT"
	case CategoryInternal:
		return "INT"
	default:
		return "UNK"
	}
}

const (
	CategorySyntax DiagnosticCategory = iota

	CategorySemantic

	CategoryType

	CategoryEmit

	CategoryInternal
)

type DiagnosticCode struct {
	Level DiagnosticLevel

	Category DiagnosticCategory

	Nr int

	Format string
}

func (d DiagnosticCode) ID() string {
	return fmt.Sprintf("%s.%s.%04d", d.Level, d.Category, d.Nr)
}

type Diagnostic struct {
	DiagnosticCode

	S TextSpan

	Msg string
}

var categoryNumbers = map[DiagnosticCategory]int{}

func template(level DiagnosticLevel, category DiagnosticCategory, fmt string) DiagnosticCode {
	if _, ok := categoryNumbers[category]; !ok {
		categoryNumbers[category] = 1
	}

	nr := categoryNumbers[category]
	categoryNumbers[category]++

	return DiagnosticCode{
		Level:    level,
		Category: category,
		Nr:       nr,
		Format:   fmt,
	}
}

func templateErr(category DiagnosticCategory, fmt string) DiagnosticCode {
	return template(LevelError, category, fmt)
}

func templateWarn(category DiagnosticCategory, fmt string) DiagnosticCode {
	return template(LevelWarning, category, fmt)
}

func templateInfo(category DiagnosticCategory, fmt string) DiagnosticCode {
	return template(LevelInfo, category, fmt)
}
