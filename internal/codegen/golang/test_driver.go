package golang

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"sova/internal/codegen"
	"sova/internal/ir"

	"github.com/dave/jennifer/jen"
)

// isTestMode reports whether the current build is a `sova test` run. Codegen branches on this to swap the wire/dev `main()` for the test driver and to unlock the special `assert` / `test` / `group` statement handlers.
func isTestMode(ctx *codegen.EmitContext) bool {
	if ctx == nil || ctx.Cache == nil {
		return false
	}
	raw, ok := ctx.Cache["build_config"]
	if !ok {
		return false
	}
	cfg, ok := raw.(interface{ TestModeValue() bool })
	return ok && cfg.TestModeValue()
}

// emitTestRuntime drops the Go-side test runtime - the `__sovaT` context type, the per-test runner, the assertion-failure recorder, and the final reporter - at the top of the generated file. Always emitted in test mode so user-authored `test "..." { ... }` bodies can reference these helpers regardless of which package they live in.
func emitTestRuntime(block *jen.Group) {
	block.Add(jen.Type().Id("__sovaTestFailure").Struct(
		jen.Id("Source").String(),
		jen.Id("Lhs").Any(),
		jen.Id("Rhs").Any(),
		jen.Id("HasOperands").Bool(),
		jen.Id("Location").String(),
		jen.Id("Vars").Map(jen.String()).Any(),
	))

	block.Add(jen.Type().Id("__sovaT").Struct(
		jen.Id("Name").String(),
		jen.Id("Failures").Index().Id("__sovaTestFailure"),
	))

	block.Add(jen.Func().Params(jen.Id("t").Op("*").Id("__sovaT")).Id("recordFailure").Params(
		jen.Id("source").String(),
		jen.Id("location").String(),
	).Block(
		jen.Id("t").Dot("Failures").Op("=").Append(jen.Id("t").Dot("Failures"), jen.Id("__sovaTestFailure").Values(jen.Dict{
			jen.Id("Source"):   jen.Id("source"),
			jen.Id("Location"): jen.Id("location"),
		})),
	))

	block.Add(jen.Func().Params(jen.Id("t").Op("*").Id("__sovaT")).Id("recordFailureV").Params(
		jen.Id("source").String(),
		jen.Id("location").String(),
		jen.Id("vars").Map(jen.String()).Any(),
	).Block(
		jen.Id("t").Dot("Failures").Op("=").Append(jen.Id("t").Dot("Failures"), jen.Id("__sovaTestFailure").Values(jen.Dict{
			jen.Id("Source"):   jen.Id("source"),
			jen.Id("Location"): jen.Id("location"),
			jen.Id("Vars"):     jen.Id("vars"),
		})),
	))

	block.Add(jen.Func().Params(jen.Id("t").Op("*").Id("__sovaT")).Id("recordCompareFailureV").Params(
		jen.Id("source").String(),
		jen.Id("lhs").Any(),
		jen.Id("rhs").Any(),
		jen.Id("location").String(),
		jen.Id("vars").Map(jen.String()).Any(),
	).Block(
		jen.Id("t").Dot("Failures").Op("=").Append(jen.Id("t").Dot("Failures"), jen.Id("__sovaTestFailure").Values(jen.Dict{
			jen.Id("Source"):      jen.Id("source"),
			jen.Id("Lhs"):         jen.Id("lhs"),
			jen.Id("Rhs"):         jen.Id("rhs"),
			jen.Id("HasOperands"): jen.True(),
			jen.Id("Location"):    jen.Id("location"),
			jen.Id("Vars"):        jen.Id("vars"),
		})),
	))

	block.Add(jen.Func().Params(jen.Id("t").Op("*").Id("__sovaT")).Id("recordComparisonFailure").Params(
		jen.Id("source").String(),
		jen.Id("lhs").Any(),
		jen.Id("rhs").Any(),
		jen.Id("location").String(),
	).Block(
		jen.Id("t").Dot("Failures").Op("=").Append(jen.Id("t").Dot("Failures"), jen.Id("__sovaTestFailure").Values(jen.Dict{
			jen.Id("Source"):      jen.Id("source"),
			jen.Id("Lhs"):         jen.Id("lhs"),
			jen.Id("Rhs"):         jen.Id("rhs"),
			jen.Id("HasOperands"): jen.True(),
			jen.Id("Location"):    jen.Id("location"),
		})),
	))

	block.Add(jen.Type().Id("__sovaTestResult").Struct(
		jen.Id("Name").String(),
		jen.Id("File").String(),
		jen.Id("Failures").Index().Id("__sovaTestFailure"),
		jen.Id("Panic").String(),
		jen.Id("DurationMS").Int64(),
	))

	block.Add(jen.Var().Id("__sovaTestCurrent").Op("*").Id("__sovaT"))
	block.Add(jen.Var().Id("__sovaFailNowSentinel").Op("=").New(jen.Struct()))

	block.Add(jen.Func().Id("__sovaRunTest").Params(
		jen.Id("name").String(),
		jen.Id("file").String(),
		jen.Id("body").Func().Params(jen.Op("*").Id("__sovaT")),
	).Id("__sovaTestResult").BlockFunc(func(g *jen.Group) {
		g.Id("__sovaTestResetHarness").Call()
		g.Id("t").Op(":=").Op("&").Id("__sovaT").Values(jen.Dict{jen.Id("Name"): jen.Id("name")})
		g.Id("__sovaTestCurrent").Op("=").Id("t")
		g.Id("start").Op(":=").Qual("time", "Now").Call()
		g.Id("res").Op(":=").Id("__sovaTestResult").Values(jen.Dict{
			jen.Id("Name"): jen.Id("name"),
			jen.Id("File"): jen.Id("file"),
		})
		g.Func().Params().Block(
			jen.Defer().Func().Params().Block(
				jen.If(jen.Id("r").Op(":=").Recover(), jen.Id("r").Op("!=").Nil()).Block(
					jen.If(jen.Id("r").Op("!=").Id("__sovaFailNowSentinel")).Block(
						jen.Id("res").Dot("Panic").Op("=").Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("r")),
					),
				),
			).Call(),
			jen.Id("body").Call(jen.Id("t")),
		).Call()
		g.Id("res").Dot("Failures").Op("=").Id("t").Dot("Failures")
		g.Id("res").Dot("DurationMS").Op("=").Qual("time", "Since").Call(jen.Id("start")).Dot("Milliseconds").Call()
		g.Id("__sovaTestCurrent").Op("=").Nil()
		g.Return(jen.Id("res"))
	}))

	block.Add(jen.Func().Id("__sovaTestFailNow").Params(jen.Id("msg").String()).Block(
		jen.If(jen.Id("__sovaTestCurrent").Op("!=").Nil()).Block(
			jen.Id("__sovaTestCurrent").Dot("recordFailure").Call(jen.Id("msg"), jen.Lit("")),
		),
		jen.Panic(jen.Id("__sovaFailNowSentinel")),
	))

	block.Add(jen.Func().Id("__sovaTestRecordFailure").Params(jen.Id("source").String(), jen.Id("location").String()).Block(
		jen.If(jen.Id("__sovaTestCurrent").Op("!=").Nil()).Block(
			jen.Id("__sovaTestCurrent").Dot("recordFailure").Call(jen.Id("source"), jen.Id("location")),
		),
	))

	block.Add(jen.Func().Id("__sovaTestRecordCompareFailure").Params(
		jen.Id("source").String(),
		jen.Id("lhs").Any(),
		jen.Id("rhs").Any(),
		jen.Id("location").String(),
	).Block(
		jen.If(jen.Id("__sovaTestCurrent").Op("!=").Nil()).Block(
			jen.Id("__sovaTestCurrent").Dot("recordComparisonFailure").Call(jen.Id("source"), jen.Id("lhs"), jen.Id("rhs"), jen.Id("location")),
		),
	))

	block.Add(jen.Func().Id("__sovaTestExpectEqual").Params(jen.Id("actual").Any(), jen.Id("expected").Any()).Bool().Block(
		jen.Return(jen.Qual("reflect", "DeepEqual").Call(jen.Id("actual"), jen.Id("expected"))),
	))

	block.Add(jen.Func().Id("__sovaTestExpectContains").Params(jen.Id("haystack").Any(), jen.Id("needle").Any()).Bool().Block(
		jen.Switch(jen.Id("h").Op(":=").Id("haystack").Assert(jen.Type())).Block(
			jen.Case(jen.String()).Block(
				jen.If(jen.List(jen.Id("n"), jen.Id("ok")).Op(":=").Id("needle").Assert(jen.String()), jen.Id("ok")).Block(
					jen.Return(jen.Qual("strings", "Contains").Call(jen.Id("h"), jen.Id("n"))),
				),
				jen.Return(jen.False()),
			),
		),
		jen.Id("rv").Op(":=").Qual("reflect", "ValueOf").Call(jen.Id("haystack")),
		jen.If(jen.Id("rv").Dot("Kind").Call().Op("==").Qual("reflect", "Slice").Op("||").Id("rv").Dot("Kind").Call().Op("==").Qual("reflect", "Array")).Block(
			jen.For(jen.Id("i").Op(":=").Lit(0), jen.Id("i").Op("<").Id("rv").Dot("Len").Call(), jen.Id("i").Op("++")).Block(
				jen.If(jen.Qual("reflect", "DeepEqual").Call(jen.Id("rv").Dot("Index").Call(jen.Id("i")).Dot("Interface").Call(), jen.Id("needle"))).Block(
					jen.Return(jen.True()),
				),
			),
		),
		jen.Return(jen.False()),
	))

	block.Add(jen.Func().Id("__sovaTestExpectThrows").Params(jen.Id("fn").Func().Params()).Bool().Block(
		jen.Id("threw").Op(":=").False(),
		jen.Func().Params().Block(
			jen.Defer().Func().Params().Block(
				jen.If(jen.Id("r").Op(":=").Recover(), jen.Id("r").Op("!=").Nil().Op("&&").Id("r").Op("!=").Id("__sovaFailNowSentinel")).Block(
					jen.Id("threw").Op("=").True(),
				),
			).Call(),
			jen.Id("fn").Call(),
		).Call(),
		jen.Return(jen.Id("threw")),
	))

	block.Add(jen.Func().Id("__sovaTestExpectCompileOutput").Params(jen.Id("src").String(), jen.Id("marker").String()).Bool().Block(
		jen.List(jen.Id("tmp"), jen.Id("err")).Op(":=").Qual("os", "CreateTemp").Call(jen.Lit(""), jen.Lit("sova-expectout-*.sova")),
		jen.If(jen.Id("err").Op("!=").Nil()).Block(jen.Return(jen.False())),
		jen.Defer().Qual("os", "Remove").Call(jen.Id("tmp").Dot("Name").Call()),
		jen.List(jen.Id("_"), jen.Id("_")).Op("=").Id("tmp").Dot("WriteString").Call(jen.Id("src")),
		jen.Id("_").Op("=").Id("tmp").Dot("Close").Call(),
		jen.List(jen.Id("outDir"), jen.Id("err2")).Op(":=").Qual("os", "MkdirTemp").Call(jen.Lit(""), jen.Lit("sova-expectout-out-*")),
		jen.If(jen.Id("err2").Op("!=").Nil()).Block(jen.Return(jen.False())),
		jen.If(jen.Id("err").Op("!=").Nil()).Block(jen.Return(jen.False())),
		jen.Defer().Qual("os", "RemoveAll").Call(jen.Id("outDir")),
		jen.Id("bin").Op(":=").Qual("os", "Getenv").Call(jen.Lit("SOVA_BIN")),
		jen.If(jen.Id("bin").Op("==").Lit("")).Block(jen.Id("bin").Op("=").Lit("sova")),
		jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Id("bin"), jen.Lit("compile"), jen.Lit("-o"), jen.Id("outDir"), jen.Id("tmp").Dot("Name").Call()),
		jen.List(jen.Id("_"), jen.Id("runErr")).Op(":=").Id("cmd").Dot("CombinedOutput").Call(),
		jen.If(jen.Id("runErr").Op("!=").Nil()).Block(jen.Return(jen.False())),
		jen.List(jen.Id("goSrc"), jen.Id("rerr")).Op(":=").Qual("os", "ReadFile").Call(jen.Qual("path/filepath", "Join").Call(jen.Id("outDir"), jen.Lit("output.go"))),
		jen.If(jen.Id("rerr").Op("==").Nil().Op("&&").Qual("strings", "Contains").Call(jen.String().Parens(jen.Id("goSrc")), jen.Id("marker"))).Block(jen.Return(jen.True())),
		jen.List(jen.Id("jsSrc"), jen.Id("rerr2")).Op(":=").Qual("os", "ReadFile").Call(jen.Qual("path/filepath", "Join").Call(jen.Id("outDir"), jen.Lit("output.js"))),
		jen.If(jen.Id("rerr2").Op("==").Nil().Op("&&").Qual("strings", "Contains").Call(jen.String().Parens(jen.Id("jsSrc")), jen.Id("marker"))).Block(jen.Return(jen.True())),
		jen.Return(jen.False()),
	))

	block.Add(jen.Func().Id("__sovaTestExpectSnapshot").Params(jen.Id("name").String(), jen.Id("value").Any()).Bool().BlockFunc(func(g *jen.Group) {
		g.List(jen.Id("payload"), jen.Id("merr")).Op(":=").Qual("encoding/json", "MarshalIndent").Call(jen.Id("value"), jen.Lit(""), jen.Lit("  "))
		g.If(jen.Id("merr").Op("!=").Nil()).Block(jen.Return(jen.False()))
		g.Id("dir").Op(":=").Qual("os", "Getenv").Call(jen.Lit("SOVA_TEST_SNAPSHOT_DIR"))
		g.If(jen.Id("dir").Op("==").Lit("")).Block(
			jen.List(jen.Id("cwd"), jen.Id("_")).Op(":=").Qual("os", "Getwd").Call(),
			jen.Id("dir").Op("=").Qual("path/filepath", "Join").Call(jen.Id("cwd"), jen.Lit(".."), jen.Lit(".sova"), jen.Lit("snapshots")),
		)
		g.If(jen.Qual("os", "MkdirAll").Call(jen.Id("dir"), jen.Lit(0o755)).Op("!=").Nil()).Block(jen.Return(jen.False()))
		g.Id("safe").Op(":=").Id("__sovaTestSnapshotSafeName").Call(jen.Id("name"))
		g.Id("snapPath").Op(":=").Qual("path/filepath", "Join").Call(jen.Id("dir"), jen.Id("safe").Op("+").Lit(".snap.json"))
		g.List(jen.Id("existing"), jen.Id("rerr")).Op(":=").Qual("os", "ReadFile").Call(jen.Id("snapPath"))
		g.If(jen.Id("rerr").Op("!=").Nil()).Block(
			jen.If(jen.Qual("os", "IsNotExist").Call(jen.Id("rerr"))).Block(
				jen.If(jen.Qual("os", "Getenv").Call(jen.Lit("SOVA_TEST_SNAPSHOT_CI")).Op("!=").Lit("")).Block(
					jen.Id("__sovaTestRecordFailure").Call(jen.Qual("fmt", "Sprintf").Call(jen.Lit("snapshot missing for %q (CI mode forbids creation)"), jen.Id("name")), jen.Id("snapPath")),
					jen.Return(jen.False()),
				),
				jen.If(jen.Qual("os", "WriteFile").Call(jen.Id("snapPath"), jen.Id("payload"), jen.Lit(0o644)).Op("!=").Nil()).Block(jen.Return(jen.False())),
				jen.Return(jen.True()),
			),
			jen.Return(jen.False()),
		)
		g.If(jen.String().Parens(jen.Id("existing")).Op("==").String().Parens(jen.Id("payload"))).Block(jen.Return(jen.True()))
		g.Id("__sovaTestRecordCompareFailure").Call(
			jen.Qual("fmt", "Sprintf").Call(jen.Lit("snapshot %q mismatch"), jen.Id("name")),
			jen.String().Parens(jen.Id("payload")),
			jen.String().Parens(jen.Id("existing")),
			jen.Id("snapPath"),
		)
		g.Return(jen.False())
	}))

	block.Add(jen.Func().Id("__sovaTestSnapshotSafeName").Params(jen.Id("s").String()).String().BlockFunc(func(g *jen.Group) {
		g.Id("buf").Op(":=").Make(jen.Index().Byte(), jen.Lit(0), jen.Qual("", "len").Call(jen.Id("s")))
		g.For(jen.List(jen.Id("_"), jen.Id("r")).Op(":=").Range().Id("s")).Block(
			jen.Switch().Block(
				jen.Case(jen.Id("r").Op(">=").LitRune('a').Op("&&").Id("r").Op("<=").LitRune('z')).Block(
					jen.Id("buf").Op("=").Append(jen.Id("buf"), jen.Byte().Parens(jen.Id("r"))),
				),
				jen.Case(jen.Id("r").Op(">=").LitRune('A').Op("&&").Id("r").Op("<=").LitRune('Z')).Block(
					jen.Id("buf").Op("=").Append(jen.Id("buf"), jen.Byte().Parens(jen.Id("r"))),
				),
				jen.Case(jen.Id("r").Op(">=").LitRune('0').Op("&&").Id("r").Op("<=").LitRune('9')).Block(
					jen.Id("buf").Op("=").Append(jen.Id("buf"), jen.Byte().Parens(jen.Id("r"))),
				),
				jen.Default().Block(
					jen.Id("buf").Op("=").Append(jen.Id("buf"), jen.LitRune('_')),
				),
			),
		)
		g.If(jen.Qual("", "len").Call(jen.Id("buf")).Op("==").Lit(0)).Block(jen.Return(jen.Lit("snap")))
		g.Return(jen.String().Parens(jen.Id("buf")))
	}))

	block.Add(jen.Func().Id("__sovaTestShouldRun").Params(jen.Id("name").String()).Bool().Block(
		jen.Id("allow").Op(":=").Qual("os", "Getenv").Call(jen.Lit("SOVA_TEST_ALLOWED")),
		jen.If(jen.Id("allow").Op("!=").Lit("")).Block(
			jen.Id("hit").Op(":=").False(),
			jen.For(jen.Id("_").Op(",").Id("n").Op(":=").Range().Qual("strings", "Split").Call(jen.Id("allow"), jen.Lit("\n"))).Block(
				jen.If(jen.Id("n").Op("==").Id("name")).Block(
					jen.Id("hit").Op("=").True(),
					jen.Break(),
				),
			),
			jen.If(jen.Op("!").Id("hit")).Block(jen.Return(jen.False())),
		),
		jen.Id("filter").Op(":=").Qual("os", "Getenv").Call(jen.Lit("SOVA_TEST_FILTER")),
		jen.If(jen.Id("filter").Op("==").Lit("")).Block(jen.Return(jen.True())),
		jen.Return(jen.Qual("strings", "Contains").Call(jen.Id("name"), jen.Id("filter"))),
	))

	block.Add(jen.Func().Id("__sovaTestJSONMode").Params().Bool().Block(
		jen.Return(jen.Qual("os", "Getenv").Call(jen.Lit("SOVA_TEST_JSON")).Op("!=").Lit("")),
	))

	block.Add(jen.Func().Id("__sovaTestEmitJSON").Params(jen.Id("r").Id("__sovaTestResult"), jen.Id("passed").Bool()).Block(
		jen.Id("status").Op(":=").Lit("pass"),
		jen.If(jen.Op("!").Id("passed")).Block(jen.Id("status").Op("=").Lit("fail")),
		jen.Id("rec").Op(":=").Map(jen.String()).Any().Values(jen.Dict{
			jen.Lit("name"):       jen.Id("r").Dot("Name"),
			jen.Lit("file"):       jen.Id("r").Dot("File"),
			jen.Lit("side"):       jen.Lit("go"),
			jen.Lit("status"):     jen.Id("status"),
			jen.Lit("durationMs"): jen.Id("r").Dot("DurationMS"),
		}),
		jen.If(jen.Id("r").Dot("Panic").Op("!=").Lit("")).Block(jen.Id("rec").Index(jen.Lit("panic")).Op("=").Id("r").Dot("Panic")),
		jen.If(jen.Qual("", "len").Call(jen.Id("r").Dot("Failures")).Op(">").Lit(0)).Block(
			jen.Id("rec").Index(jen.Lit("failures")).Op("=").Id("r").Dot("Failures"),
		),
		jen.List(jen.Id("buf"), jen.Id("err")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("rec")),
		jen.If(jen.Id("err").Op("==").Nil()).Block(
			jen.Qual("fmt", "Println").Call(jen.String().Parens(jen.Id("buf"))),
		),
	))

	block.Add(jen.Func().Id("__sovaTestReport").Params(jen.Id("results").Index().Id("__sovaTestResult")).Int().Block(
		jen.Id("pass").Op(":=").Lit(0),
		jen.Id("fail").Op(":=").Lit(0),
		jen.Id("jsonMode").Op(":=").Id("__sovaTestJSONMode").Call(),
		jen.For(jen.Id("_").Op(",").Id("r").Op(":=").Range().Id("results")).Block(
			jen.Id("passed").Op(":=").Qual("", "len").Call(jen.Id("r").Dot("Failures")).Op("==").Lit(0).Op("&&").Id("r").Dot("Panic").Op("==").Lit(""),
			jen.If(jen.Id("passed")).Block(jen.Id("pass").Op("++")).Else().Block(jen.Id("fail").Op("++")),
			jen.If(jen.Id("jsonMode")).Block(
				jen.Id("__sovaTestEmitJSON").Call(jen.Id("r"), jen.Id("passed")),
				jen.Continue(),
			),
			jen.If(jen.Id("passed")).Block(
				jen.Qual("fmt", "Printf").Call(jen.Lit("PASS  %s  (%dms)\n"), jen.Id("r").Dot("Name"), jen.Id("r").Dot("DurationMS")),
			).Else().Block(
				jen.Qual("fmt", "Printf").Call(jen.Lit("FAIL  %s  (%dms)\n"), jen.Id("r").Dot("Name"), jen.Id("r").Dot("DurationMS")),
				jen.If(jen.Id("r").Dot("Panic").Op("!=").Lit("")).Block(
					jen.Qual("fmt", "Printf").Call(jen.Lit("      panic: %s\n"), jen.Id("r").Dot("Panic")),
				),
				jen.For(jen.Id("_").Op(",").Id("fl").Op(":=").Range().Id("r").Dot("Failures")).Block(
					jen.If(jen.Id("fl").Dot("HasOperands")).Block(
						jen.Qual("fmt", "Printf").Call(jen.Lit("      assert failed at %s\n        assert %s\n        lhs = %#v\n        rhs = %#v\n"),
							jen.Id("fl").Dot("Location"), jen.Id("fl").Dot("Source"), jen.Id("fl").Dot("Lhs"), jen.Id("fl").Dot("Rhs")),
					).Else().Block(
						jen.Qual("fmt", "Printf").Call(jen.Lit("      assert failed at %s\n        assert %s\n"),
							jen.Id("fl").Dot("Location"), jen.Id("fl").Dot("Source")),
					),
					jen.If(jen.Qual("", "len").Call(jen.Id("fl").Dot("Vars")).Op(">").Lit(0)).Block(
						jen.Id("__names").Op(":=").Make(jen.Index().String(), jen.Lit(0), jen.Qual("", "len").Call(jen.Id("fl").Dot("Vars"))),
						jen.For(jen.Id("k").Op(":=").Range().Id("fl").Dot("Vars")).Block(
							jen.Id("__names").Op("=").Append(jen.Id("__names"), jen.Id("k")),
						),
						jen.Qual("sort", "Strings").Call(jen.Id("__names")),
						jen.For(jen.Id("_").Op(",").Id("k").Op(":=").Range().Id("__names")).Block(
							jen.Qual("fmt", "Printf").Call(jen.Lit("        %s = %#v\n"), jen.Id("k"), jen.Id("fl").Dot("Vars").Index(jen.Id("k"))),
						),
					),
				),
			),
		),
		jen.If(jen.Op("!").Id("jsonMode")).Block(
			jen.Qual("fmt", "Printf").Call(jen.Lit("\n%d passed, %d failed\n"), jen.Id("pass"), jen.Id("fail")),
		),
		jen.If(jen.Id("fail").Op(">").Lit(0)).Block(jen.Return(jen.Lit(1))),
		jen.Return(jen.Lit(0)),
	))
}

// hashTestName produces a deterministic, Go-identifier-safe suffix from a fully qualified test path (group chain + leaf name). Used to name the per-test driver function emitted alongside the test body.
func hashTestName(pkgPath string, groupPath []string, name string) string {
	h := sha1.New()
	h.Write([]byte(pkgPath))
	for _, g := range groupPath {
		h.Write([]byte("/"))
		h.Write([]byte(g))
	}
	h.Write([]byte("/"))
	h.Write([]byte(name))
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// fullTestName builds the human-readable test name including any enclosing group path, used in reporter output.
func fullTestName(groupPath []string, name string) string {
	if len(groupPath) == 0 {
		return name
	}
	return strings.Join(groupPath, " > ") + " > " + name
}

// emitAssertStmt lowers a Sova `assert <expr>` statement to a runtime check inside the current test function. Comparison-shaped expressions (`==`, `!=`, `<`, `<=`, `>`, `>=`) split into LHS/RHS so the reporter can show both values; everything else lands in a bare-boolean form with only the source text. Power-assert: every distinct VarRef in the asserted expression is captured into a `map[string]any` and attached to the failure record so the reporter can print `var = value` lines under the failed assertion.
func (e *CodeEmitter) emitAssertStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, block *jen.Group, s *ir.AssertStmt) {
	span := s.Span()
	location := fmt.Sprintf("%s:%d:%d", span.File, span.StartLn, span.StartCol)
	source := assertSourceText(ctx, pkg, f, s.Expr)
	vars := collectAssertVars(ctx, pkg, f, s.Expr)
	buildVarsMap := func() jen.Code {
		entries := jen.Dict{}
		for _, v := range vars {
			entries[jen.Lit(v.OrigName)] = e.buildExpr(ctx, pkg, f, v.Expr)
		}
		return jen.Map(jen.String()).Any().Values(entries)
	}
	if bin, ok := s.Expr.(*ir.BinaryExpr); ok && isComparisonOp(bin.Op) {
		e.withStmt(block, func() jen.Code {
			lhs := e.buildExpr(ctx, pkg, f, bin.Left)
			rhs := e.buildExpr(ctx, pkg, f, bin.Right)
			cond := e.buildExpr(ctx, pkg, f, s.Expr)
			return jen.Func().Params().Block(
				jen.Id("__lhs").Op(":=").Add(lhs),
				jen.Id("__rhs").Op(":=").Add(rhs),
				jen.Id("_").Op("=").Id("__lhs"),
				jen.Id("_").Op("=").Id("__rhs"),
				jen.If(jen.Op("!").Parens(cond)).Block(
					jen.Id("__t").Dot("recordCompareFailureV").Call(
						jen.Lit(source),
						jen.Id("__lhs"),
						jen.Id("__rhs"),
						jen.Lit(location),
						buildVarsMap(),
					),
				),
			).Call()
		})
		return
	}
	e.withStmt(block, func() jen.Code {
		cond := e.buildExpr(ctx, pkg, f, s.Expr)
		return jen.If(jen.Op("!").Parens(cond)).Block(
			jen.Id("__t").Dot("recordFailureV").Call(jen.Lit(source), jen.Lit(location), buildVarsMap()),
		)
	})
}

type capturedVar struct {
	OrigName string
	Expr     ir.Expr
}

// collectAssertVars walks an assert expression and collects each distinct VarRef as a captured var for the power-assert report. The display name is the original Sova identifier (pre-mangle); duplicates are kept only once.
func collectAssertVars(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, expr ir.Expr) []capturedVar {
	seen := map[string]bool{}
	var out []capturedVar
	var walk func(ir.Expr)
	walk = func(e ir.Expr) {
		if e == nil {
			return
		}
		switch x := e.(type) {
		case *ir.VarRef:
			name := x.Ref.Name
			if name == "" || seen[name] {
				return
			}
			if pkg != nil && x.Ref.Sym != 0 {
				if sym, ok := pkg.Syms.GetByID(x.Ref.Sym); ok {
					if sym.Kind != ir.SK_Variable {
						return
					}
				}
			}
			seen[name] = true
			out = append(out, capturedVar{OrigName: name, Expr: x})
		case *ir.BinaryExpr:
			walk(x.Left)
			walk(x.Right)
		case *ir.UnaryExpr:
			walk(x.Expr)
		case *ir.PrefixUnaryExpr:
			walk(x.Expr)
		case *ir.PostfixUnaryExpr:
			walk(x.Expr)
		case *ir.GroupedExpr:
			walk(x.Expr)
		case *ir.CoalesceExpr:
			walk(x.Left)
			walk(x.Default)
		case *ir.TenaryExpr:
			walk(x.Cond)
			walk(x.Then)
			walk(x.Else)
		case *ir.IndexExpr:
			walk(x.Expr)
			walk(x.Index)
		case *ir.FieldAccessExpr:
			walk(x.Expr)
		case *ir.FuncCallExpr:
			walk(x.Callee)
			for _, a := range x.Args {
				walk(a.Expr)
			}
		}
	}
	walk(expr)
	return out
}

// emitAsSessionStmt lowers `asSession("name") { ... }` to a Go block that installs the named test session as the current `@` for the duration of the body, then restores the previous session. An empty Name (`asSession() { ... }`) creates a fresh anonymous session each entry. Only meaningful inside `on test` builds; the named-session registry lives in the test harness runtime.
func (e *CodeEmitter) emitAsSessionStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, block *jen.Group, s *ir.AsSessionStmt) {
	name := s.Name
	block.Add(jen.Func().Params().BlockFunc(func(g *jen.Group) {
		g.Id("__prevSession").Op(":=").Id("__sovaCurrentSession").Call()
		g.Id("__sovaTestSwitchSession").Call(jen.Lit(name))
		g.Defer().Func().Params().Block(
			jen.If(jen.Id("__prevSession").Op("!=").Nil()).Block(
				jen.Id("__sovaSetCurrentSession").Call(jen.Id("__prevSession")),
			).Else().Block(
				jen.Id("__sovaClearCurrentSession").Call(),
			),
		).Call()
		if s.Body != nil {
			e.emitBlock(ctx, pkg, f, g, s.Body.Stmts)
		}
	}).Call())
}

// isComparisonOp reports whether the given binary operator produces a bool comparable result the assert reporter can split into LHS/RHS values.
func isComparisonOp(op ir.Op) bool {
	switch op {
	case ir.OpEq, ir.OpNeq, ir.OpLt, ir.OpLte, ir.OpGt, ir.OpGte:
		return true
	}
	return false
}

// assertSourceText returns a best-effort source snippet of the asserted expression. Uses the raw file content sliced by the expression's span when available, otherwise falls back to a synthesised representation.
func assertSourceText(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, expr ir.Expr) string {
	if expr == nil {
		return ""
	}
	span := expr.Span()
	if span.File == "" || f == nil {
		return ""
	}
	for _, file := range pkg.Files {
		if file.Filename != span.File {
			continue
		}
		content := file.Content
		lines := strings.Split(content, "\n")
		if span.StartLn-1 < 0 || span.StartLn-1 >= len(lines) {
			return ""
		}
		line := lines[span.StartLn-1]
		startCol := span.StartCol - 1
		endCol := span.EndCol - 1
		if startCol < 0 {
			startCol = 0
		}
		if endCol > len(line) {
			endCol = len(line)
		}
		if endCol > startCol {
			return line[startCol:endCol]
		}
		return strings.TrimSpace(line)
	}
	return ""
}

// testModeFromCache mirrors `isTestMode`, kept under this name because callers in `emitter.go` already reference it.
func testModeFromCache(ctx *codegen.EmitContext) bool {
	return isTestMode(ctx)
}

// emitTestDriverMain writes only the test driver `main()` body (the per-test functions and runtime helpers are emitted separately at the top of the file). Reads `[]codegen.TestRegistryEntryView` from the shared `test_registry_view` cache key. Tests that opt into parallel execution (the test decl itself or any enclosing group carries `parallel`) are spawned into goroutines and joined via `sync.WaitGroup`; sequential tests run inline in declaration order. Mixed suites first run all sequential tests, then the parallel ones, then report - keeping sequential output stable while still benefiting from concurrency where requested.
func emitTestDriverMain(ctx *codegen.EmitContext, g *jen.Group) {
	entries := readTestRegistryView(ctx)
	if len(entries) == 0 {
		g.Qual("fmt", "Println").Call(jen.Lit("no tests discovered"))
		return
	}

	hasParallel := false
	for _, entry := range entries {
		if entry.Parallel {
			hasParallel = true
			break
		}
	}

	g.Id("__results").Op(":=").Make(jen.Index().Id("__sovaTestResult"), jen.Lit(0), jen.Lit(len(entries)))
	if hasParallel {
		g.Var().Id("__resultsMu").Qual("sync", "Mutex")
		g.Var().Id("__wg").Qual("sync", "WaitGroup")
		g.Id("_").Op("=").Op("&").Id("__resultsMu")
	}
	for _, entry := range entries {
		funcName := "__sovaTestImpl_" + hashTestName(entry.Pkg.Path.String(), entry.GroupPath, entry.Decl.Name)
		fullName := fullTestName(entry.GroupPath, entry.Decl.Name)
		file := ""
		if entry.File != nil {
			file = entry.File.Filename
		}
		if entry.Parallel {
			continue
		}
		g.If(jen.Id("__sovaTestShouldRun").Call(jen.Lit(fullName))).Block(
			jen.Id("__results").Op("=").Append(jen.Id("__results"),
				jen.Id("__sovaRunTest").Call(jen.Lit(fullName), jen.Lit(file), jen.Id(funcName)),
			),
		)
	}
	if hasParallel {
		for _, entry := range entries {
			if !entry.Parallel {
				continue
			}
			funcName := "__sovaTestImpl_" + hashTestName(entry.Pkg.Path.String(), entry.GroupPath, entry.Decl.Name)
			fullName := fullTestName(entry.GroupPath, entry.Decl.Name)
			file := ""
			if entry.File != nil {
				file = entry.File.Filename
			}
			g.If(jen.Id("__sovaTestShouldRun").Call(jen.Lit(fullName))).Block(
				jen.Id("__wg").Dot("Add").Call(jen.Lit(1)),
				jen.Go().Func().Params().Block(
					jen.Defer().Id("__wg").Dot("Done").Call(),
					jen.Id("__r").Op(":=").Id("__sovaRunTest").Call(jen.Lit(fullName), jen.Lit(file), jen.Id(funcName)),
					jen.Id("__resultsMu").Dot("Lock").Call(),
					jen.Id("__results").Op("=").Append(jen.Id("__results"), jen.Id("__r")),
					jen.Id("__resultsMu").Dot("Unlock").Call(),
				).Call(),
			)
		}
		g.Id("__wg").Dot("Wait").Call()
	}
	g.Qual("os", "Exit").Call(jen.Id("__sovaTestReport").Call(jen.Id("__results")))
}

// emitTestImplFuncs emits one Go function per discovered test plus the global per-group setupAll/teardownAll helpers. Each test function: fires the inherited setupAlls via sync.Once (keyed on the SetupStmt's IR NodeID so all tests in the owning group share the gate), runs per-test setups, the test body, per-test teardowns. teardownAlls are NOT fired here - `emitTestDriverMain` runs them once globally at suite exit.
func (e *CodeEmitter) emitTestImplFuncs(ctx *codegen.EmitContext, block *jen.Group) {
	entries := readTestRegistryView(ctx)

	setupAllSeen := map[ir.NodeID]bool{}
	teardownAllSeen := map[ir.NodeID]bool{}
	for _, entry := range entries {
		for _, s := range entry.SetupAlls {
			if setupAllSeen[s.ID()] {
				continue
			}
			setupAllSeen[s.ID()] = true
			s := s
			ent := entry
			bodyName := setupAllBodyName(s)
			block.Add(jen.Func().Id(bodyName).Params().BlockFunc(func(g *jen.Group) {
				if s.Body != nil {
					for _, st := range s.Body.Stmts {
						e.emitStmt(ctx, ent.Pkg, ent.File.Hir, g, st, false)
					}
				}
			}))
		}
		for _, t := range entry.TeardownAlls {
			if teardownAllSeen[t.ID()] {
				continue
			}
			teardownAllSeen[t.ID()] = true
			t := t
			ent := entry
			bodyName := teardownAllBodyName(t)
			block.Add(jen.Func().Id(bodyName).Params().BlockFunc(func(g *jen.Group) {
				if t.Body != nil {
					for _, st := range t.Body.Stmts {
						e.emitStmt(ctx, ent.Pkg, ent.File.Hir, g, st, false)
					}
				}
			}))
		}
	}

	for _, entry := range entries {
		entry := entry
		funcName := "__sovaTestImpl_" + hashTestName(entry.Pkg.Path.String(), entry.GroupPath, entry.Decl.Name)
		block.Add(jen.Func().Id(funcName).Params(jen.Id("__t").Op("*").Id("__sovaT")).BlockFunc(func(g *jen.Group) {
			for _, s := range entry.SetupAlls {
				g.Id(setupAllBodyName(s)).Call()
			}
			emitBody := func(stmts []ir.Stmt) {
				for _, st := range stmts {
					e.emitStmt(ctx, entry.Pkg, entry.File.Hir, g, st, false)
					g.Id("__sovaTestAwaitWires").Call()
				}
			}
			for _, body := range entry.SetupBodies {
				emitBody(body)
			}
			if entry.Decl.Body != nil {
				emitBody(entry.Decl.Body.Stmts)
			}
			for _, body := range entry.TeardownBodies {
				emitBody(body)
			}
			for _, t := range entry.TeardownAlls {
				g.Id(teardownAllBodyName(t)).Call()
			}
		}))
	}
}

func setupAllBodyName(s *ir.SetupStmt) string {
	return fmt.Sprintf("__sovaSetupAllBody_%d", uint64(s.ID()))
}

func teardownAllBodyName(t *ir.TeardownStmt) string {
	return fmt.Sprintf("__sovaTeardownAllBody_%d", uint64(t.ID()))
}

func readTestRegistryView(ctx *codegen.EmitContext) []codegen.TestRegistryEntryView {
	if ctx == nil || ctx.Cache == nil {
		return nil
	}
	raw, ok := ctx.Cache[codegen.TestRegistryViewCacheKey]
	if !ok {
		return nil
	}
	if entries, ok := raw.([]codegen.TestRegistryEntryView); ok {
		return entries
	}
	return nil
}
