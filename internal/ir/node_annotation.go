package ir

import "sova/internal/diag"

type Annotation struct {
	Name         NameRef
	Args         []Expr
	ArgNames     []string
	ResolvedArgs []AnnotationValue
}

type AnnotationValueKind int

const (
	AnnotationValueUnknown AnnotationValueKind = iota
	AnnotationValueString
	AnnotationValueInt
	AnnotationValueBool
)

type AnnotationValue struct {
	Kind AnnotationValueKind
	Str  string
	Int  int64
	Bool bool
}

type EmbedKind int

const (
	EmbedKindUnknown EmbedKind = iota
	EmbedKindText
	EmbedKindBytes
)

type EmbedInfo struct {
	SourcePath  string
	Kind        EmbedKind
	ContentHash string
	SizeBytes   int64
	Span        diag.TextSpan
}

type AssetInfo struct {
	SourcePath  string
	ContentHash string
	URL         string
	StagedName  string
	SizeBytes   int64
	Span        diag.TextSpan
}
