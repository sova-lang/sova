# Sova Cleanup Tracker

Living checklist of simplification and architectural cleanups. Items are roughly ordered by recommended sequence (low-risk file refactors first, deep architectural changes later). Strike out items as they land; add new ones at the bottom as new pain points emerge.

---

## LSP simplifications (most code volume per file)

- [ ] **L1 — Handler boilerplate wrapper.** Every LSP handler (hover, definition, rename, references, signature_help) starts with the same dance (`snap.Snapshot()` → `Compile()` → `findCursorTarget()`). Extract to `s.withCursor(params, func(c, target) (X, error))`. Expected: -150 LOC across 10+ handlers.
- [ ] **L2 — Centralise position math.** `offsetToLSPPosition`, `lineColToOffset`, `lspPositionToOffset`, `spanToLSPRange` are scattered across `completion.go`, `text_sync.go`, `position_lookup.go`. Consolidate into one `positions.go` (skeleton already exists). Add UTF-16-awareness while you're there.
- [ ] **L3 — Split `completion.go` (1 786 LOC, 28 funcs).** Pure file-split:
  - `completion_classify.go` — cursor-context detection (`classifyCompletion`, `isInsideWireOptions`, `isInsideImportString`, `isInsideGenericString`, etc.)
  - `completion_sources.go` — per-kind item builders (annotations, synths, wire options, identifiers)
  - `completion_members.go` — dot-access resolution (`memberCompletions`, `findExprTypeEndingAt`, `findEnclosingThisType`)
  - Top-level `Completion()` stays as dispatcher in `completion.go`.
- [ ] **L4 — Replace `ir_walker.go` cursor walker with a general visitor.** 679 LOC of LSP-specific tree walking that overlaps with every analysis pass's own walker. Hard-blocks: depends on **C2** below.
- [ ] **L5 — Investigate `cssclasses.go` (898 LOC).** One feature, very large. Likely 2-3 concerns mixed (parser, classifier, diagnostics, completion). Spike-then-split.

## Compiler-wide simplifications

- [ ] **C1 — Annotation resolver framework.** `pass_resolve_embeds.go` and `pass_resolve_assets.go` are ~90% identical (find annotation by name, validate relative path, read file, report errors). Extract a shared resolver with pluggable validators + unified `ErrAsset…` / `ErrEmbed…` codes. Future-proofs new `@font`, `@svg`, etc. without copy-paste.
- [ ] **C2 — IR visitor / pass-iteration helper.** 17+ passes reimplement the same outer loop:
  ```go
  for _, pkg := range pc.Pkgs { for _, f := range pkg.Files { for _, st := range f.Hir.Statements { ... }}}
  ```
  Extract `passes.IterateStatements(pc, opts, fn)` with options (`IncludeSynth`, `IncludeTransitive`). Same primitive becomes the basis for an `ir.Visitor` interface that L4 builds on.
- [ ] **C3 — Split monolithic emitter switches into per-case methods.** Pure mechanical refactor; same volume, drastically smaller mental load:
  - [internal/codegen/golang/emitter.go:1735](internal/codegen/golang/emitter.go#L1735) — `buildExpr`: 33 cases, 664 lines
  - [internal/codegen/golang/emitter.go:381](internal/codegen/golang/emitter.go#L381) — `emitStmt`: 29 cases, 1 354 lines
  - [internal/codegen/javascript/emitter_exprs.go:55](internal/codegen/javascript/emitter_exprs.go#L55) — `buildExpr`: 33 cases, 529 lines
  - [internal/passes/pass_infer_types.go:1523](internal/passes/pass_infer_types.go#L1523) — `synthesizeTypeFromExpr`: 1 120 lines
  Pattern: keep top-level dispatcher (~30 lines), one method per case (~30–80 lines), each isolated and unit-testable.
- [ ] **C4 — Cross-emitter dispatch trait.** After C3, the Go and JS emitters have parallel per-case methods. Define a `codegen.NodeEmitter` interface so the dispatcher itself is shared; backends only override per-node-type behaviour. Saves ~1 500 LOC of parallel switch maintenance.

## Architecture (lower-volume but higher leverage)

- [x] **A2 — Split `internal/ir/node.go` (1 100 LOC, 118 types).** Pure file-split, zero behaviour change. Categories: `node_decl.go`, `node_stmt.go`, `node_expr.go`, `node_type.go`, `node_annotation.go`, `node_wire.go`. Base types (`node`, `exprBase`, `docBase`) stay in `node.go`.
- [ ] **A5 — Codegen backend registry.** `internal/passes/pass_codegen.go` imports `internal/codegen/golang` and `internal/codegen/javascript` directly. Replace with a `codegen.Register(name, factory)` registry the backends call from `init()`; pass_codegen iterates the registry. Trivial change, opens the door to a third backend.
- [ ] **A4 — Move NameMap (mangle output) out of IR.** Today `internal/ir/symbol.go:272` holds `toMangled`/`toOriginal` filled by `pass_mangle` and read by codegen — IR carries pass-specific output. Move to `PassContext` so each backend can have its own mangling scheme without racing on IR state.
- [ ] **A3 — Move pass-result metadata off IR nodes.** `VarDeclStmt.Embed`, `.Asset`, `.Wire` are populated by specific passes and read only by codegen. Today every new annotation pass adds another field to `VarDeclStmt`. Replace with side tables on `PassContext` keyed by `NodeID`:
  ```go
  type PassContext struct {
      EmbedInfo map[NodeID]*EmbedInfo
      AssetInfo map[NodeID]*AssetInfo
      WireInfo  map[NodeID]*WireSpec
      ...
  }
  ```
  IR stays clean; new annotation passes don't touch IR types.
- [ ] **A1 — Split `ir.Type` god-object (32 fields, 1 321 external refs).** Discriminated tagged-union: `Type { ID; Kind; Info TypeInfo }` with variant-specific info structs (`StructInfo`, `EnumInfo`, `FuncInfo`, `InterfaceInfo`, `ExternInfo`, `PrimitiveInfo`). Large migration — schedule as a dedicated branch with extensive test bring-up first. **Do not bundle with anything else.**

---

## Recommended sequencing

1. **This week (low-risk, visible wins):** A2 → L2 → L1 → A5
2. **Next week (mechanical, broad reach):** L3 → C1 → C2
3. **Following sprint (heavier):** A3 → A4 → L4 (depends on C2)
4. **Dedicated branch (long migration):** C3 → C4 → A1

## Out of scope (for now)

- Performance work on the type inferer.
- Generating LSP handlers from a schema.
- LSP protocol upgrades (incremental sync, semantic tokens delta, etc.).
- Rewriting the ANTLR parser.

## Notes & decisions

_Add dated notes as you make trade-off decisions during the cleanup, so future you (or a future contributor) knows why._
