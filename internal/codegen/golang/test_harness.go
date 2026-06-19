package golang

import (
	"sova/internal/codegen"

	"github.com/dave/jennifer/jen"
)

func emitTestHarnessStubs(block *jen.Group) {
	block.Add(jen.Func().Id("__sovaTestHarnessActive").Params().Bool().Block(jen.Return(jen.False())))
	block.Add(jen.Func().Id("__sovaTestNow").Params().Qual("time", "Time").Block(jen.Return(jen.Qual("time", "Now").Call())))
	block.Add(jen.Func().Id("__sovaTestRandom").Params().Int64().Block(jen.Return(jen.Qual("time", "Now").Call().Dot("UnixNano").Call().Op("&").Lit(int64(0x7FFFFFFFFFFFFFFF)))))
	block.Add(jen.Func().Id("__sovaTestRegisterGracePurge").Params(jen.Id("sid").String(), jen.Id("graceSec").Int()).Block())
	block.Add(jen.Func().Id("__sovaTestDisconnect").Params().Block())
	block.Add(jen.Func().Id("__sovaTestAdvanceTime").Params(jen.Id("seconds").Float64()).Block())
	block.Add(jen.Func().Id("__sovaTestAwaitWires").Params().Block())
	block.Add(jen.Func().Id("__sovaTestUseRealTime").Params().Block())
	block.Add(jen.Func().Id("__sovaTestUseRealRandom").Params().Block())
	block.Add(jen.Func().Id("__sovaTestMockRegister").Params(jen.Id("name").String(), jen.Id("retVal").Any(), jen.Id("hasReturn").Bool()).Block())
	block.Add(jen.Func().Id("__sovaTestMockCall").Params(jen.Id("name").String(), jen.Id("args").Op("...").Any()).Params(jen.Any(), jen.Bool()).Block(jen.Return(jen.Nil(), jen.False())))
	block.Add(jen.Func().Id("__sovaTestMockCallCount").Params(jen.Id("name").String()).Int().Block(jen.Return(jen.Lit(0))))
	block.Add(jen.Func().Id("__sovaTestMockLastArgs").Params(jen.Id("name").String()).Index().Any().Block(jen.Return(jen.Nil())))
	block.Add(jen.Func().Id("__sovaTestMockReset").Params().Block())
	block.Add(jen.Func().Id("__sovaTestMockTime").Params(jen.Id("unixSecs").Int64()).Block())
	block.Add(jen.Func().Id("__sovaTestMockRandom").Params(jen.Id("seed").Int64()).Block())
	block.Add(jen.Func().Id("__sovaTestExpectCompileError").Params(jen.Id("src").String()).Bool().Block(jen.Return(jen.False())))
	block.Add(jen.Func().Id("__sovaTestExpectCompileOutput").Params(jen.Id("src").String(), jen.Id("marker").String()).Bool().Block(jen.Return(jen.False())))
	block.Add(jen.Func().Id("__sovaTestExpectSnapshot").Params(jen.Id("name").String(), jen.Id("value").Any()).Bool().Block(jen.Return(jen.True())))
	block.Add(jen.Func().Id("__sovaTestExpectEqual").Params(jen.Id("a").Any(), jen.Id("b").Any()).Bool().Block(jen.Return(jen.Qual("reflect", "DeepEqual").Call(jen.Id("a"), jen.Id("b")))))
	block.Add(jen.Func().Id("__sovaTestExpectContains").Params(jen.Id("h").Any(), jen.Id("n").Any()).Bool().Block(jen.Return(jen.False())))
	block.Add(jen.Func().Id("__sovaTestExpectThrows").Params(jen.Id("fn").Func().Params()).Bool().Block(jen.Return(jen.False())))
	block.Add(jen.Func().Id("__sovaTestFailNow").Params(jen.Id("msg").String()).Block())
	block.Add(jen.Func().Id("__sovaMockHook").Params(jen.Id("name").String(), jen.Id("args").Op("...").Any()).Params(jen.Any(), jen.Bool(), jen.Bool()).Block(jen.Return(jen.Nil(), jen.False(), jen.False())))
}

func emitTestHarness(ctx *codegen.EmitContext, block *jen.Group) {
	if !isTestMode(ctx) {
		return
	}

	block.Add(jen.Type().Id("__sovaTestGraceEntry").Struct(
		jen.Id("sid").String(),
		jen.Id("fireAt").Qual("time", "Time"),
	))

	block.Add(jen.Type().Id("__sovaMockCall").Struct(
		jen.Id("Args").Index().Any(),
	))

	block.Add(jen.Type().Id("__sovaMockEntry").Struct(
		jen.Id("Calls").Index().Id("__sovaMockCall"),
		jen.Id("Return").Any(),
		jen.Id("HasReturn").Bool(),
	))

	block.Add(jen.Var().Id("__sovaTestHarness").Op("=").Struct(
		jen.Id("mu").Qual("sync", "Mutex"),
		jen.Id("sessions").Map(jen.String()).Op("*").Id("fn____Session"),
		jen.Id("clockOffset").Qual("time", "Duration"),
		jen.Id("realTime").Bool(),
		jen.Id("realRandom").Bool(),
		jen.Id("graceEntries").Index().Id("__sovaTestGraceEntry"),
		jen.Id("mocks").Map(jen.String()).Op("*").Id("__sovaMockEntry"),
		jen.Id("randSeq").Uint64(),
	).Values(jen.Dict{
		jen.Id("sessions"): jen.Map(jen.String()).Op("*").Id("fn____Session").Values(),
		jen.Id("mocks"):    jen.Map(jen.String()).Op("*").Id("__sovaMockEntry").Values(),
	}))

	block.Add(jen.Func().Id("__sovaTestHarnessActive").Params().Bool().Block(
		jen.Return(jen.True()),
	))

	block.Add(jen.Func().Id("__sovaTestNow").Params().Qual("time", "Time").Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Id("off").Op(":=").Id("__sovaTestHarness").Dot("clockOffset"),
		jen.Id("real").Op(":=").Id("__sovaTestHarness").Dot("realTime"),
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
		jen.If(jen.Id("real")).Block(jen.Return(jen.Qual("time", "Now").Call())),
		jen.Return(jen.Qual("time", "Now").Call().Dot("Add").Call(jen.Id("off"))),
	))

	block.Add(jen.Func().Id("__sovaTestAdvanceTime").Params(jen.Id("seconds").Float64()).Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Id("__sovaTestHarness").Dot("clockOffset").Op("+=").Qual("time", "Duration").Parens(jen.Id("seconds").Op("*").Float64().Parens(jen.Qual("time", "Second"))),
		jen.Id("now").Op(":=").Qual("time", "Now").Call().Dot("Add").Call(jen.Id("__sovaTestHarness").Dot("clockOffset")),
		jen.Var().Id("expired").Index().String(),
		jen.Var().Id("keep").Index().Id("__sovaTestGraceEntry"),
		jen.For(jen.Id("_").Op(",").Id("e").Op(":=").Range().Id("__sovaTestHarness").Dot("graceEntries")).Block(
			jen.If(jen.Op("!").Id("e").Dot("fireAt").Dot("After").Call(jen.Id("now"))).Block(
				jen.Id("expired").Op("=").Append(jen.Id("expired"), jen.Id("e").Dot("sid")),
			).Else().Block(
				jen.Id("keep").Op("=").Append(jen.Id("keep"), jen.Id("e")),
			),
		),
		jen.Id("__sovaTestHarness").Dot("graceEntries").Op("=").Id("keep"),
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
		jen.For(jen.Id("_").Op(",").Id("sid").Op(":=").Range().Id("expired")).Block(
			jen.Id("__sovaRunGracePurge").Call(jen.Id("sid")),
		),
	))

	block.Add(jen.Func().Id("__sovaTestRegisterGracePurge").Params(jen.Id("sid").String(), jen.Id("graceSec").Int()).Block(
		jen.Id("fireAt").Op(":=").Id("__sovaTestNow").Call().Dot("Add").Call(jen.Qual("time", "Duration").Call(jen.Id("graceSec")).Op("*").Qual("time", "Second")),
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Id("__sovaTestHarness").Dot("graceEntries").Op("=").Append(jen.Id("__sovaTestHarness").Dot("graceEntries"), jen.Id("__sovaTestGraceEntry").Values(jen.Dict{
			jen.Id("sid"):    jen.Id("sid"),
			jen.Id("fireAt"): jen.Id("fireAt"),
		})),
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
	))

	block.Add(jen.Func().Id("__sovaTestUseRealTime").Params().Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Id("__sovaTestHarness").Dot("realTime").Op("=").True(),
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
	))

	block.Add(jen.Func().Id("__sovaTestUseRealRandom").Params().Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Id("__sovaTestHarness").Dot("realRandom").Op("=").True(),
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
	))

	block.Add(jen.Func().Id("__sovaTestGetOrCreateSession").Params(jen.Id("name").String()).Op("*").Id("fn____Session").Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
		jen.If(jen.Id("name").Op("!=").Lit("")).Block(
			jen.If(jen.List(jen.Id("s"), jen.Id("ok")).Op(":=").Id("__sovaTestHarness").Dot("sessions").Index(jen.Id("name")), jen.Id("ok")).Block(
				jen.Return(jen.Id("s")),
			),
		),
		jen.Var().Id("id").String(),
		jen.If(jen.Id("__sovaTestHarness").Dot("realRandom")).Block(
			jen.Id("id").Op("=").Id("__sovaNewSessionId").Call(),
		).Else().Block(
			jen.Id("__sovaTestHarness").Dot("randSeq").Op("++"),
			jen.Id("id").Op("=").Qual("fmt", "Sprintf").Call(jen.Lit("test%016x"), jen.Id("__sovaTestHarness").Dot("randSeq")),
		),
		jen.Id("s").Op(":=").Op("&").Id("fn____Session").Values(jen.Dict{
			jen.Id("Id"):          jen.Id("id"),
			jen.Id("IsConnected"): jen.True(),
			jen.Id("ConnectedAt"): jen.Qual("time", "Now").Call().Dot("Unix").Call(),
		}),
		jen.Id("__sovaSessionPut").Call(jen.Id("s")),
		jen.If(jen.Id("name").Op("!=").Lit("")).Block(
			jen.Id("__sovaTestHarness").Dot("sessions").Index(jen.Id("name")).Op("=").Id("s"),
		),
		jen.Return(jen.Id("s")),
	))

	block.Add(jen.Func().Id("__sovaTestSwitchSession").Params(jen.Id("name").String()).Block(
		jen.Id("s").Op(":=").Id("__sovaTestGetOrCreateSession").Call(jen.Id("name")),
		jen.Id("__sovaSetCurrentSession").Call(jen.Id("s")),
	))

	block.Add(jen.Func().Id("__sovaTestDisconnect").Params().Block(
		jen.Id("s").Op(":=").Id("__sovaCurrentSession").Call(),
		jen.If(jen.Id("s").Op("==").Nil()).Block(jen.Return()),
		jen.Id("__sovaSessionRegistry").Dot("mu").Dot("Lock").Call(),
		jen.Id("s").Dot("IsConnected").Op("=").False(),
		jen.Id("__sovaSessionRegistry").Dot("mu").Dot("Unlock").Call(),
		jen.Id("__sovaScheduleGracePurge").Call(jen.Id("s").Dot("Id")),
	))

	block.Add(jen.Func().Id("__sovaTestAwaitWires").Params().Block(
		jen.Comment("Auto-await barrier: yields once so any pending outbox writers get a chance to flush. The full WS-delivery tracking is a follow-up."),
		jen.Qual("runtime", "Gosched").Call(),
	))

	block.Add(jen.Func().Id("__sovaTestResetHarness").Params().Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Id("__sovaTestHarness").Dot("sessions").Op("=").Map(jen.String()).Op("*").Id("fn____Session").Values(),
		jen.Id("__sovaTestHarness").Dot("clockOffset").Op("=").Lit(0),
		jen.Id("__sovaTestHarness").Dot("realTime").Op("=").False(),
		jen.Id("__sovaTestHarness").Dot("realRandom").Op("=").False(),
		jen.Id("__sovaTestHarness").Dot("graceEntries").Op("=").Nil(),
		jen.Id("__sovaTestHarness").Dot("mocks").Op("=").Map(jen.String()).Op("*").Id("__sovaMockEntry").Values(),
		jen.Id("__sovaTestHarness").Dot("randSeq").Op("=").Lit(0),
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
		jen.Id("__sovaSessionRegistry").Dot("mu").Dot("Lock").Call(),
		jen.Id("__sovaSessionRegistry").Dot("m").Op("=").Map(jen.String()).Op("*").Id("fn____Session").Values(),
		jen.Id("__sovaSessionRegistry").Dot("mu").Dot("Unlock").Call(),
		jen.Id("__sovaClearCurrentSession").Call(),
	))

	block.Add(jen.Func().Id("__sovaTestMockRegister").Params(jen.Id("name").String(), jen.Id("retVal").Any(), jen.Id("hasReturn").Bool()).Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
		jen.Id("__sovaTestHarness").Dot("mocks").Index(jen.Id("name")).Op("=").Op("&").Id("__sovaMockEntry").Values(jen.Dict{
			jen.Id("Return"):    jen.Id("retVal"),
			jen.Id("HasReturn"): jen.Id("hasReturn"),
		}),
	))

	block.Add(jen.Func().Id("__sovaTestMockCall").Params(jen.Id("name").String(), jen.Id("args").Op("...").Any()).Params(jen.Any(), jen.Bool()).Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
		jen.List(jen.Id("entry"), jen.Id("ok")).Op(":=").Id("__sovaTestHarness").Dot("mocks").Index(jen.Id("name")),
		jen.If(jen.Op("!").Id("ok")).Block(jen.Return(jen.Nil(), jen.False())),
		jen.Id("entry").Dot("Calls").Op("=").Append(jen.Id("entry").Dot("Calls"), jen.Id("__sovaMockCall").Values(jen.Dict{
			jen.Id("Args"): jen.Id("args"),
		})),
		jen.Return(jen.Id("entry").Dot("Return"), jen.Id("entry").Dot("HasReturn")),
	))

	block.Add(jen.Func().Id("__sovaTestMockCallCount").Params(jen.Id("name").String()).Int().Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
		jen.List(jen.Id("entry"), jen.Id("ok")).Op(":=").Id("__sovaTestHarness").Dot("mocks").Index(jen.Id("name")),
		jen.If(jen.Op("!").Id("ok")).Block(jen.Return(jen.Lit(0))),
		jen.Return(jen.Qual("", "len").Call(jen.Id("entry").Dot("Calls"))),
	))

	block.Add(jen.Func().Id("__sovaTestMockLastArgs").Params(jen.Id("name").String()).Index().Any().Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
		jen.List(jen.Id("entry"), jen.Id("ok")).Op(":=").Id("__sovaTestHarness").Dot("mocks").Index(jen.Id("name")),
		jen.If(jen.Op("!").Id("ok").Op("||").Qual("", "len").Call(jen.Id("entry").Dot("Calls")).Op("==").Lit(0)).Block(jen.Return(jen.Nil())),
		jen.Return(jen.Id("entry").Dot("Calls").Index(jen.Qual("", "len").Call(jen.Id("entry").Dot("Calls")).Op("-").Lit(1)).Dot("Args")),
	))

	block.Add(jen.Func().Id("__sovaMockHook").Params(jen.Id("name").String(), jen.Id("args").Op("...").Any()).Params(jen.Any(), jen.Bool(), jen.Bool()).Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
		jen.List(jen.Id("entry"), jen.Id("ok")).Op(":=").Id("__sovaTestHarness").Dot("mocks").Index(jen.Id("name")),
		jen.If(jen.Op("!").Id("ok")).Block(jen.Return(jen.Nil(), jen.False(), jen.False())),
		jen.Id("entry").Dot("Calls").Op("=").Append(jen.Id("entry").Dot("Calls"), jen.Id("__sovaMockCall").Values(jen.Dict{
			jen.Id("Args"): jen.Id("args"),
		})),
		jen.Return(jen.Id("entry").Dot("Return"), jen.Id("entry").Dot("HasReturn"), jen.True()),
	))

	block.Add(jen.Func().Id("__sovaTestMockRandom").Params(jen.Id("seed").Int64()).Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Id("__sovaTestHarness").Dot("randSeq").Op("=").Uint64().Parens(jen.Id("seed")),
		jen.Id("__sovaTestHarness").Dot("realRandom").Op("=").False(),
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
	))

	block.Add(jen.Func().Id("__sovaTestRandom").Params().Int64().Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
		jen.If(jen.Id("__sovaTestHarness").Dot("realRandom")).Block(
			jen.Return(jen.Qual("time", "Now").Call().Dot("UnixNano").Call()),
		),
		jen.Comment("Splitmix64 step: deterministic, well-distributed, no extra state."),
		jen.Id("__sovaTestHarness").Dot("randSeq").Op("+=").Lit(uint64(0x9E3779B97F4A7C15)),
		jen.Id("z").Op(":=").Id("__sovaTestHarness").Dot("randSeq"),
		jen.Id("z").Op("=").Parens(jen.Id("z").Op("^").Parens(jen.Id("z").Op(">>").Lit(30))).Op("*").Lit(uint64(0xBF58476D1CE4E5B9)),
		jen.Id("z").Op("=").Parens(jen.Id("z").Op("^").Parens(jen.Id("z").Op(">>").Lit(27))).Op("*").Lit(uint64(0x94D049BB133111EB)),
		jen.Id("z").Op("=").Id("z").Op("^").Parens(jen.Id("z").Op(">>").Lit(31)),
		jen.Return(jen.Int64().Parens(jen.Id("z").Op("&").Lit(uint64(0x7FFFFFFFFFFFFFFF)))),
	))

	block.Add(jen.Func().Id("__sovaTestMockReset").Params().Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
		jen.Id("__sovaTestHarness").Dot("mocks").Op("=").Map(jen.String()).Op("*").Id("__sovaMockEntry").Values(),
	))

	block.Add(jen.Func().Id("__sovaTestMockTime").Params(jen.Id("unixSecs").Int64()).Block(
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Lock").Call(),
		jen.Id("__sovaTestHarness").Dot("clockOffset").Op("=").Qual("time", "Unix").Call(jen.Id("unixSecs"), jen.Lit(0)).Dot("Sub").Call(jen.Qual("time", "Now").Call()),
		jen.Id("__sovaTestHarness").Dot("mu").Dot("Unlock").Call(),
	))

	block.Add(jen.Func().Id("__sovaTestExpectCompileError").Params(jen.Id("src").String()).Bool().Block(
		jen.List(jen.Id("tmp"), jen.Id("err")).Op(":=").Qual("os", "CreateTemp").Call(jen.Lit(""), jen.Lit("sova-expect-*.sova")),
		jen.If(jen.Id("err").Op("!=").Nil()).Block(jen.Return(jen.False())),
		jen.Defer().Qual("os", "Remove").Call(jen.Id("tmp").Dot("Name").Call()),
		jen.List(jen.Id("_"), jen.Id("_")).Op("=").Id("tmp").Dot("WriteString").Call(jen.Id("src")),
		jen.Id("_").Op("=").Id("tmp").Dot("Close").Call(),
		jen.Id("bin").Op(":=").Qual("os", "Getenv").Call(jen.Lit("SOVA_BIN")),
		jen.If(jen.Id("bin").Op("==").Lit("")).Block(jen.Id("bin").Op("=").Lit("sova")),
		jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Id("bin"), jen.Lit("check"), jen.Id("tmp").Dot("Name").Call()),
		jen.List(jen.Id("_"), jen.Id("runErr")).Op(":=").Id("cmd").Dot("CombinedOutput").Call(),
		jen.Return(jen.Id("runErr").Op("!=").Nil()),
	))
}
