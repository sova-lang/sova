package diag

import (
	"fmt"
)

// TextSpan represents a span of text in a source file. It is used for diagnostics and error reporting.
type TextSpan struct {
	File     string // File is the name of the file where the text span is located. If empty, the file is unknown.
	StartLn  int    // StartLn is the starting line number of the text span, starting from 1.
	StartCol int    // StartCol is the starting column number of the text span, starting from 1.
	EndLn    int    // EndLn is the ending line number of the text span, starting from 1.
	EndCol   int    // EndCol is the ending column number of the text span, starting from 1.
}

func (s TextSpan) String() string {
	if s.File == "" {
		return fmt.Sprintf("(%d:%d-%d:%d)", s.StartLn, s.StartCol, s.EndLn, s.EndCol)
	}
	return fmt.Sprintf("%s:(%d:%d-%d:%d)", s.File, s.StartLn, s.StartCol, s.EndLn, s.EndCol)
}

// DiagnosticLevel indicates the severity of a diagnostic message.
type DiagnosticLevel int

// String returns the string representation of the DiagnosticLevel.
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
	// LevelError indicates a critical issue that prevents compilation.
	LevelError DiagnosticLevel = iota
	// LevelWarning indicates a potential issue that does not prevent compilation.
	LevelWarning
	// LevelInfo provides informational messages that may be useful for debugging or understanding the code.
	LevelInfo
)

// DiagnosticCategory represents the category of a diagnostic message, such as syntax errors, type errors, etc.
type DiagnosticCategory int

// String returns the string representation of the DiagnosticCategory.
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
	// CategorySyntax indicates a syntax-related diagnostic message.
	CategorySyntax DiagnosticCategory = iota
	// CategorySemantic indicates a semantic-related diagnostic message.
	CategorySemantic
	// CategoryType indicates a type-related diagnostic message.
	CategoryType
	// CategoryEmit indicates a diagnostic message related to code emission or generation.
	CategoryEmit
	// CategoryInternal indicates an internal error that should not occur in normal operation.
	CategoryInternal
)

// DiagnosticCode represents a diagnostic message template.
type DiagnosticCode struct {
	// Level indicates the severity of the diagnostic message.
	Level DiagnosticLevel
	// Category indicates the category of the diagnostic message.
	Category DiagnosticCategory
	// Nr is an numeric code which number uniquely identifies the diagnostic message.
	Nr int
	// Format is the diagnostic message. It uses printf-style formatting.
	Format string
}

// ID returns the formatted diagnostic code ID.
func (d DiagnosticCode) ID() string {
	return fmt.Sprintf("%s.%s.%04d", d.Level, d.Category, d.Nr)
}

// Diagnostic is a diagnostic message that includes a code, source span, and formatted message.
type Diagnostic struct {
	DiagnosticCode
	// S is the source span of the diagnostic message, used for error reporting.
	S TextSpan
	// Msg is the formatted message of the diagnostic, which may include additional context.
	Msg string
}

var categoryNumbers = map[DiagnosticCategory]int{}

// template creates a new DiagnosticCode with the given level, category, and message.
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

// templateErr creates a new DiagnosticCode for an error message.
func templateErr(category DiagnosticCategory, fmt string) DiagnosticCode {
	return template(LevelError, category, fmt)
}

// templateWarn creates a new DiagnosticCode for a warning message.
func templateWarn(category DiagnosticCategory, fmt string) DiagnosticCode {
	return template(LevelWarning, category, fmt)
}

// templateInfo creates a new DiagnosticCode for an informational message.
func templateInfo(category DiagnosticCategory, fmt string) DiagnosticCode {
	return template(LevelInfo, category, fmt)
}
