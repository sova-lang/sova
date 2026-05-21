package diag

import (
	"github.com/antlr4-go/antlr/v4"
)

// AntlrErrorListener is a custom error listener for ANTLR that implements the antlr.ErrorListener interface and collects
// error messages during parsing transforming them into the Sova diagnostic format.
type AntlrErrorListener struct {
	*antlr.DefaultErrorListener
	filename string // The name of the file being parsed, used for diagnostics.
	diag     *DiagnosticsBag
}

// NewAntlrErrorListener creates a new instance of AntlrErrorListener with the provided DiagnosticsBag.
func NewAntlrErrorListener(filename string, diag *DiagnosticsBag) *AntlrErrorListener {
	return &AntlrErrorListener{
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
		filename:             filename,
		diag:                 diag,
	}
}

func (d *AntlrErrorListener) SyntaxError(_ antlr.Recognizer, offendingSymbol any, ln, col int, msg string, _ antlr.RecognitionException) {
	commonToken, ok := offendingSymbol.(*antlr.CommonToken)
	if ok && commonToken.GetTokenType() == antlr.TokenEOF {
		d.diag.Report(ErrUnexpectedEOF, TextSpan{
			File:     d.filename,
			StartLn:  ln,
			StartCol: col,
			EndLn:    ln,
			EndCol:   col + 1,
		})
	} else {
		d.diag.Report(ErrUnexpectedToken, TextSpan{
			File:     d.filename,
			StartLn:  ln,
			StartCol: col,
			EndLn:    ln,
			EndCol:   col + 1,
		}, msg)
	}
}
