package jsgen

import (
	"encoding/json"
	"strings"
)

type SourcePosition struct {
	Line       int
	Column     int
	SourceFile string
}

type Mapping struct {
	GeneratedLine   int
	GeneratedColumn int
	SourceFile      string
	OriginalLine    int
	OriginalColumn  int
}

type SourceMap struct {
	Version        int      `json:"version"`
	File           string   `json:"file"`
	SourceRoot     string   `json:"sourceRoot"`
	Sources        []string `json:"sources"`
	SourcesContent []string `json:"sourcesContent,omitempty"`
	Names          []string `json:"names"`
	Mappings       string   `json:"mappings"`
}

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

func (b *SourceMapBuilder) AddSourceContent(sourceFile string, content string) {
	b.sourcesContent[sourceFile] = content
}

func (b *SourceMapBuilder) getSourceIndex(sourceFile string) int {
	if idx, ok := b.sourceIndexMap[sourceFile]; ok {
		return idx
	}

	idx := len(b.sources)
	b.sources = append(b.sources, sourceFile)
	b.sourceIndexMap[sourceFile] = idx
	return idx
}

func (b *SourceMapBuilder) AddMapping(sourceFile string, origLine, origCol int) {
	if sourceFile == "" {
		return
	}

	b.mappings = append(b.mappings, Mapping{
		GeneratedLine:   b.currentGenLine,
		GeneratedColumn: b.currentGenCol,
		SourceFile:      sourceFile,
		OriginalLine:    origLine,
		OriginalColumn:  origCol,
	})
}

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
			lastGenCol = 0
		}

		if currentLine > 1 && result.Len() > 0 && result.String()[result.Len()-1] != ';' {
			result.WriteByte(',')
		}

		sourceIndex := b.getSourceIndex(m.SourceFile)

		result.WriteString(encodeVLQ(m.GeneratedColumn - lastGenCol))
		result.WriteString(encodeVLQ(sourceIndex - lastSourceIndex))
		result.WriteString(encodeVLQ(m.OriginalLine - 1 - lastOrigLine))
		result.WriteString(encodeVLQ(m.OriginalColumn - lastOrigCol))

		lastGenCol = m.GeneratedColumn
		lastSourceIndex = sourceIndex
		lastOrigLine = m.OriginalLine - 1
		lastOrigCol = m.OriginalColumn
	}

	return result.String()
}

const vlqBase64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

const (
	vlqBaseShift       = 5
	vlqBase            = 1 << vlqBaseShift
	vlqBaseMask        = vlqBase - 1
	vlqContinuationBit = vlqBase
)

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

func (sm *SourceMap) ToJSON() (string, error) {
	data, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}
