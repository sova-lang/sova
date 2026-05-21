package passes

import "sova/internal/ir"

// PassPopulateBuiltins seeds the codegen-side `builtin_intrinsics` cache and the `builtin_error_typ` cache from the symbols declared in `std/__globals__`. The actual built-in declarations live in `std/__globals__.sova` as ordinary Sova source so users can navigate to them; this pass just teaches the rest of the compiler which SymIDs trigger intrinsic dispatch (print → fmt.Println, len → host len, error → *sovaError construction, ...).
type PassPopulateBuiltins struct{}

func (p *PassPopulateBuiltins) Name() string       { return "populate_builtins" }
func (p *PassPopulateBuiltins) Scope() PassScope   { return PerBuild }
func (p *PassPopulateBuiltins) Requires() []string { return []string{"infer_types"} }
func (p *PassPopulateBuiltins) NoErrors() bool     { return false }

var builtinIntrinsicNames = map[string]struct{}{
	"print":   {},
	"println": {},
	"len":     {},
	"error":   {},
	"after":   {},
	"every":   {},
}

const (
	builtinIntrinsicsCacheKey = "builtin_intrinsics"
	builtinErrorTypeCacheKey  = "builtin_error_typ"
)

func (p *PassPopulateBuiltins) Run(pc *PassContext) error {
	var globals *ir.PackageContext
	for _, pkg := range pc.Pkgs {
		if pkg.Path.String() == "std/__globals__" {
			globals = pkg
			break
		}
	}
	if globals == nil {
		return nil
	}
	intrinsics, _ := pc.Cache[builtinIntrinsicsCacheKey].(map[ir.SymID]string)
	if intrinsics == nil {
		intrinsics = map[ir.SymID]string{}
		pc.Cache[builtinIntrinsicsCacheKey] = intrinsics
	}
	for _, f := range globals.Files {
		if f.Hir == nil {
			continue
		}
		for _, st := range f.Hir.Statements {
			switch s := st.(type) {
			case *ir.FuncDeclStmt:
				if _, isBuiltin := builtinIntrinsicNames[s.Name.Name]; !isBuiltin {
					continue
				}
				if s.Name.Sym == 0 {
					continue
				}
				intrinsics[s.Name.Sym] = s.Name.Name
				pc.Names.Add(s.Name.Sym, s.Name.Name, s.Name.Name)
			case *ir.TypeDeclStmt:
				if s.Name.Name == "error" && s.Name.Sym != 0 {
					if sym, ok := globals.Syms.GetByID(s.Name.Sym); ok && sym.Typ != 0 {
						pc.Cache[builtinErrorTypeCacheKey] = sym.Typ
					}
				}
			}
		}
	}
	return nil
}
