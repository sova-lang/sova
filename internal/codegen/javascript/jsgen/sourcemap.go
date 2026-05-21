package jsgen

import (
	"encoding/json"
	"strings"
)

// SourcePosition represents a position in the original source code
type SourcePosition struct {
	Line       int    // 1-based line number
	Column     int    // 0-based column number
	SourceFile string // Source file name
}

// Mapping represents a single source map mapping
type Mapping struct {
	GeneratedLine   int
	GeneratedColumn int
	SourceFile      string
	OriginalLine    int
	OriginalColumn  int
}

// SourceMap represents a source map (v3 format)
type SourceMap struct {
	Version        int      `json:"version"`
	File           string   `json:"file"`
	SourceRoot     string   `json:"sourceRoot"`
	Sources        []string `json:"sources"`
	SourcesContent []string `json:"sourcesContent,omitempty"`
	Names          []string `json:"names"`
	Mappings       string   `json:"mappings"`
}

// SourceMapBuilder builds a source map
type SourceMapBuilder struct {
	outputFile      string
	sources         []string
	sourceIndexMap  map[string]int
	sourcesContent  map[string]string
	mappings        []Mapping
	currentGenLine  int
	currentGenCol   int
	lastGenLine     int
	lastSourceIndex int
	lastOrigLine    int
	lastOrigCol     int
}

// NewSourceMapBuilder creates a new source map builder
func NewSourceMapBuilder(outputFile string) *SourceMapBuilder {
	return &SourceMapBuilder{
		outputFile:     outputFile,
		sources:        make([]string, 0),
		sourceIndexMap: make(map[string]int),
		sourcesContent: make(map[string]string),
		mappings:       make([]Mapping, 0),
		currentGenLine: 1,
		currentGenCol:  0,
		lastGenLine:    1,
	}
}

// AddSourceContent adds the content of a source file
func (b *SourceMapBuilder) AddSourceContent(sourceFile string, content string) {
	b.sourcesContent[sourceFile] = content
}

// getSourceIndex returns the index of a source file, adding it if necessary
func (b *SourceMapBuilder) getSourceIndex(sourceFile string) int {
	if idx, ok := b.sourceIndexMap[sourceFile]; ok {
		return idx
	}
	idx := len(b.sources)
	b.sources = append(b.sources, sourceFile)
	b.sourceIndexMap[sourceFile] = idx
	return idx
}

// AddMapping adds a mapping at the current generated position
func (b *SourceMapBuilder) AddMapping(sourceFile string, origLine, origCol int) {
	if sourceFile == "" {
		return // No source position
	}

	b.mappings = append(b.mappings, Mapping{
		GeneratedLine:   b.currentGenLine,
		GeneratedColumn: b.currentGenCol,
		SourceFile:      sourceFile,
		OriginalLine:    origLine,
		OriginalColumn:  origCol,
	})
}

// AdvanceGeneratedPosition advances the current generated position
func (b *SourceMapBuilder) AdvanceGeneratedPosition(generatedCode string) {
	for _, ch := range generatedCode {
		if ch == '\n' {
			b.currentGenLine++
			b.currentGenCol = 0
		} else {
			b.currentGenCol++
		}
	}
}

// Build generates the source map
func (b *SourceMapBuilder) Build() *SourceMap {
	mappingsStr := b.encodeMappings()

	var sourcesContent []string
	if len(b.sourcesContent) > 0 {
		sourcesContent = make([]string, len(b.sources))
		for i, src := range b.sources {
			sourcesContent[i] = b.sourcesContent[src]
		}
	}

	return &SourceMap{
		Version:        3,
		File:           b.outputFile,
		SourceRoot:     "",
		Sources:        b.sources,
		SourcesContent: sourcesContent,
		Names:          []string{},
		Mappings:       mappingsStr,
	}
}

// encodeMappings encodes the mappings using VLQ encoding
func (b *SourceMapBuilder) encodeMappings() string {
	if len(b.mappings) == 0 {
		return ""
	}

	var result strings.Builder
	lastGenCol := 0
	lastSourceIndex := 0
	lastOrigLine := 0
	lastOrigCol := 0

	currentLine := 1
	for _, m := range b.mappings {
		for currentLine < m.GeneratedLine {
			result.WriteByte(';')
			currentLine++
			lastGenCol = 0 // Reset column offset for new line
		}

		if currentLine > 1 && result.Len() > 0 && result.String()[result.Len()-1] != ';' {
			result.WriteByte(',')
		}

		sourceIndex := b.getSourceIndex(m.SourceFile)

		// Encode segment: [genCol, sourceIndex, origLine, origCol]
		result.WriteString(encodeVLQ(m.GeneratedColumn - lastGenCol))
		result.WriteString(encodeVLQ(sourceIndex - lastSourceIndex))
		result.WriteString(encodeVLQ(m.OriginalLine - 1 - lastOrigLine)) // Convert to 0-based
		result.WriteString(encodeVLQ(m.OriginalColumn - lastOrigCol))

		lastGenCol = m.GeneratedColumn
		lastSourceIndex = sourceIndex
		lastOrigLine = m.OriginalLine - 1 // Store as 0-based
		lastOrigCol = m.OriginalColumn
	}

	return result.String()
}

// VLQ encoding alphabet
const vlqBase64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

const (
	vlqBaseShift       = 5
	vlqBase            = 1 << vlqBaseShift
	vlqBaseMask        = vlqBase - 1
	vlqContinuationBit = vlqBase
)

// encodeVLQ encodes an integer using Variable Length Quantity (VLQ) encoding
func encodeVLQ(value int) string {
	var result strings.Builder

	var vlq int
	if value < 0 {
		vlq = ((-value) << 1) | 1
	} else {
		vlq = value << 1
	}

	for {
		digit := vlq & vlqBaseMask
		vlq >>= vlqBaseShift

		if vlq > 0 {
			digit |= vlqContinuationBit
		}

		result.WriteByte(vlqBase64[digit])

		if vlq == 0 {
			break
		}
	}

	return result.String()
}

// ToJSON converts the source map to JSON
func (sm *SourceMap) ToJSON() (string, error) {
	data, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
