# Known issues surfaced by the language test suite

## Open

### Generics — return type erases to `any`
`func identity<T>(x: T): T` compiles, but in Go the result is typed `any` and holds `int64`, while the assertion compares against an untyped `int`. `any(int64(42)) == int(42)` is `false`. Per `CLAUDE.md`, generics are "not yet implemented" — this isn't a bug to fix here; it's a feature still being built.

## Fixed in this session
| # | Bug | Commit |
|---|---|---|
| 1 | `customWireHandlerRegistry` emitted with unresolved `http_Request`/`http_Response` when std/http not loaded | `fix(codegen/go): skip customWireHandler registry emission when std/http isn't loaded` |
| 2 | `sova test` driver build failed under workspace mode (`go.work` interferes with `.output/go.mod`) | `fix(cli/test): set GOWORK=off when building test driver in .output` |
| 3 | char literal emitted as Go string, not rune | `fix(codegen/go): emit char literals as Go rune literals, not strings` |
| 4 | for-range Go inclusive vs JS exclusive divergence | `fix(codegen/go): make for-range end exclusive to match JS backend` |
| 5 | assert recorder emits untyped `nil` for `none` and leaks builtin `len()` mangled fn name | `fix(codegen/go): type assert lhs/rhs as any and skip callee in var capture` |
| 6 | `detect_unused` skipped in test pipeline + didn't descend into test bodies | `fix(passes): run detect_unused in test pipeline and descend into test bodies` |
| 7 | `tag` reserved by test grammar — couldn't be used as field/method/var name | `fix(grammar): make 'tag' a soft-id so it can be used as field/method/var name` |
| 8 | JS for-in destructure `[_,_]` rejected by JS (duplicate `_`) | `fix(codegen/js): avoid duplicate _ in for-in destructure when both vars unused` |
| 9 | `option<int> = 42` boxed as `*int` instead of `*int64` | `fix(codegen/go): type option box temp explicitly so &t matches *ElemType` |
| 10 | Multi-mixin composition — turned out to be #7 in disguise (`tag` field) | (resolved by fix 7) |
| 11 | Cross-package import in JS test mode — `compute_reachability` didn't walk `AssertStmt`/`AsSessionStmt`, so referenced symbols got pruned by DCE | `fix(passes): walk AssertStmt and AsSessionStmt in compute_reachability` |
| 12 | for-int statement `for let i = 0; i < N; i++` — lexer skipped `;`, so the grammar's `';'` literals could never match. Switched separator to `,` and rewired through resolve_names/infer_types/Go codegen | `fix(grammar): use ',' for for-int separators (lexer skips ';') + wire through passes` |

For-int form is now: `for let i = 0, i < N, i++ { ... }` — note commas. The `let` keyword is required to disambiguate from `for x in coll`.
