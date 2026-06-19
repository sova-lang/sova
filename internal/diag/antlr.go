package diag

import (
	"github.com/antlr4-go/antlr/v4"
)

type AntlrErrorListener struct {
	*antlr.DefaultErrorListener
	filename string
	diag     *DiagnosticsBag
}

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
