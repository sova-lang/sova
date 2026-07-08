# Known issues surfaced by the language test suite

## Open

_None currently — all previously-known issues have been fixed._

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
| 10 | Multi-mixin composition — was #7 in disguise (`tag` field) | (resolved by fix 7) |
| 11 | Cross-package import in JS test mode — `compute_reachability` didn't walk `AssertStmt`/`AsSessionStmt`, so referenced symbols got pruned by DCE | `fix(passes): walk AssertStmt and AsSessionStmt in compute_reachability` |
| 12 | for-int statement `for let i = 0; i < N; i++` — lexer skipped `;`, so grammar's `';'` literals could never match. Switched separator to `,` and wired through resolve_names/infer_types/Go codegen | `fix(grammar): use ',' for for-int separators (lexer skips ';') + wire through passes` |
| 13 | Generic function returning bare `T` — call site typed result as `any`, breaking comparison with concrete literals. Now infers T → concrete and emits `.(T)` type assertion | `fix(generics): infer T at call site + emit type assertion for bare T returns` |
| 14 | `option<T>` → `any` widening didn't deref at codegen — extern bodies received `*any` instead of the underlying value, so `x.(string)` always failed | `fix(codegen/go): auto-deref option<T> when widening to any at call sites` |
| 15 | Generic composite return types (`[]T`, `map<K,V>`, `option<T>`) — Sova call type was left as `[]any` etc.; assignment / indexing broke. Now substitutes composite return types AND emits a per-element cast loop at the call site | `fix(generics): substitute + cast composite return types ([]T, map<K,V>, option<T>)` |
| 16 | `IndexAssignmentStmt` (`m[k] = v`) didn't clear the unused flag on `k`/`v` — generic function params referenced only via index-assign got emitted as `_` in Go, breaking the body | (same commit as 15) |

For-int form: `for let i = 0, i < N, i++ { ... }` — commas, and `let` required.
