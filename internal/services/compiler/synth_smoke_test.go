package compiler

import (
	"sova/internal/ir"
	"strings"
	"testing"
)

func TestSynthPhase1ExpandsFieldAnnotation(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth GormPK on field F {
    emit on F {
        @structTag("gorm", "primaryKey")
    }
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

type User {
    @GormPK
    id: int
    name: string
}
`)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg, ok := c.Packages["myApp"]
	if !ok {
		t.Fatalf("package myApp not found")
	}
	var userType *ir.TypeDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if td, ok := st.(*ir.TypeDeclStmt); ok && td.Name.Name == "User" {
				userType = td
			}
		}
	}
	if userType == nil {
		t.Fatalf("type User not found")
	}
	var idField *ir.TypeField
	for _, fld := range userType.Fields {
		if fld.Name.Name == "id" {
			idField = fld
		}
	}
	if idField == nil {
		t.Fatalf("field id not found")
	}
	if len(idField.Annotations) != 1 {
		t.Fatalf("id field: want exactly 1 annotation after expansion, got %d (%+v)", len(idField.Annotations), idField.Annotations)
	}
	a := idField.Annotations[0]
	if a.Name.Name != "structTag" {
		t.Fatalf("id field annotation: want structTag, got %s", a.Name.Name)
	}
	if len(a.ResolvedArgs) != 2 {
		t.Fatalf("structTag: want 2 resolved args, got %d", len(a.ResolvedArgs))
	}
	if a.ResolvedArgs[0].Kind != ir.AnnotationValueString || a.ResolvedArgs[0].Str != "gorm" {
		t.Fatalf("structTag arg0: want string \"gorm\", got %+v", a.ResolvedArgs[0])
	}
	if a.ResolvedArgs[1].Kind != ir.AnnotationValueString || a.ResolvedArgs[1].Str != "primaryKey" {
		t.Fatalf("structTag arg1: want string \"primaryKey\", got %+v", a.ResolvedArgs[1])
	}
}

func TestSynthPhase2SubstitutesParams(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth Column(name: string) on field F {
    emit on F {
        @structTag("gorm", "column:" + name)
    }
}

synth Validate(rule: string) on field F {
    emit on F {
        @structTag("validate", ` + "`${rule}`" + `)
    }
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

type User {
    @Column("user_id")
    id: int

    @Validate("required")
    name: string
}
`)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["myApp"]
	if pkg == nil {
		t.Fatalf("package myApp missing")
	}
	var user *ir.TypeDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if td, ok := st.(*ir.TypeDeclStmt); ok && td.Name.Name == "User" {
				user = td
			}
		}
	}
	if user == nil {
		t.Fatalf("type User missing")
	}
	fields := map[string]*ir.TypeField{}
	for _, fld := range user.Fields {
		fields[fld.Name.Name] = fld
	}
	expectTag := func(fieldName, wantKey, wantVal string) {
		t.Helper()
		fld := fields[fieldName]
		if fld == nil {
			t.Fatalf("field %s missing", fieldName)
		}
		if len(fld.Annotations) != 1 {
			t.Fatalf("field %s: want 1 annotation, got %d", fieldName, len(fld.Annotations))
		}
		a := fld.Annotations[0]
		if a.Name.Name != "structTag" {
			t.Fatalf("field %s: want structTag, got %s", fieldName, a.Name.Name)
		}
		if len(a.ResolvedArgs) != 2 {
			t.Fatalf("field %s: want 2 resolved args, got %d", fieldName, len(a.ResolvedArgs))
		}
		if a.ResolvedArgs[0].Str != wantKey {
			t.Fatalf("field %s tag key: want %q, got %q", fieldName, wantKey, a.ResolvedArgs[0].Str)
		}
		if a.ResolvedArgs[1].Str != wantVal {
			t.Fatalf("field %s tag value: want %q, got %q", fieldName, wantVal, a.ResolvedArgs[1].Str)
		}
	}
	expectTag("id", "gorm", "column:user_id")
	expectTag("name", "validate", "required")
}

func TestSynthPhase3ExpandsTypeFuncAndLetTargets(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth Table(name: string) on type T {
    emit on T {
        @meta("table", name)
    }
}

synth Route(method: string, path: string) on func G {
    emit on G {
        @meta("route", method + " " + path)
    }
}

synth Reactive on let L {
    emit on L {
        @reactive
    }
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

@Table("users")
type User {
    id: int
}

@Route("GET", "/hello")
func hello() {
}

@Reactive
let counter: int = 0
`)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["myApp"]
	if pkg == nil {
		t.Fatalf("package myApp missing")
	}
	var (
		userType   *ir.TypeDeclStmt
		helloFunc  *ir.FuncDeclStmt
		counterLet *ir.VarDeclStmt
	)
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			switch s := st.(type) {
			case *ir.TypeDeclStmt:
				if s.Name.Name == "User" {
					userType = s
				}
			case *ir.FuncDeclStmt:
				if s.Name.Name == "hello" {
					helloFunc = s
				}
			case *ir.VarDeclStmt:
				if len(s.Targets) == 1 && s.Targets[0].Name != nil && s.Targets[0].Name.Name == "counter" {
					counterLet = s
				}
			}
		}
	}
	if userType == nil || helloFunc == nil || counterLet == nil {
		t.Fatalf("missing decls: user=%v hello=%v counter=%v", userType != nil, helloFunc != nil, counterLet != nil)
	}

	if len(userType.Annotations) != 1 || userType.Annotations[0].Name.Name != "meta" {
		t.Fatalf("User type: want single @meta annotation, got %+v", userType.Annotations)
	}
	args := userType.Annotations[0].ResolvedArgs
	if len(args) != 2 || args[0].Str != "table" || args[1].Str != "users" {
		t.Fatalf("User @meta args: want [\"table\", \"users\"], got %+v", args)
	}

	if len(helloFunc.Annotations) != 1 || helloFunc.Annotations[0].Name.Name != "meta" {
		t.Fatalf("hello func: want single @meta annotation, got %+v", helloFunc.Annotations)
	}
	args = helloFunc.Annotations[0].ResolvedArgs
	if len(args) != 2 || args[0].Str != "route" || args[1].Str != "GET /hello" {
		t.Fatalf("hello @meta args: want [\"route\", \"GET /hello\"], got %+v", args)
	}

	if len(counterLet.Annotations) != 1 || counterLet.Annotations[0].Name.Name != "reactive" {
		t.Fatalf("counter let: want single @reactive annotation, got %+v", counterLet.Annotations)
	}
}

func TestSynthPhase4FixpointChains(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth Pk on field F {
    emit on F {
        @Unique
    }
}

synth Unique on field F {
    emit on F {
        @structTag("gorm", "primaryKey;unique")
    }
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

type User {
    @Pk
    id: int
}
`)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["myApp"]
	var fld *ir.TypeField
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if td, ok := st.(*ir.TypeDeclStmt); ok && td.Name.Name == "User" {
				for _, fl := range td.Fields {
					if fl.Name.Name == "id" {
						fld = fl
					}
				}
			}
		}
	}
	if fld == nil {
		t.Fatalf("field id missing")
	}
	if len(fld.Annotations) != 1 || fld.Annotations[0].Name.Name != "structTag" {
		t.Fatalf("expected @Pk → @Unique → @structTag chain, got %+v", fld.Annotations)
	}
	args := fld.Annotations[0].ResolvedArgs
	if len(args) != 2 || args[0].Str != "gorm" || args[1].Str != "primaryKey;unique" {
		t.Fatalf("chained structTag args wrong: %+v", args)
	}
}

func TestSynthPhase4RecursionLimitDiagnoses(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth Loop on field F {
    emit on F {
        @Loop
    }
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

type T {
    @Loop
    x: int
}
`)
	_ = c.Check()
	if !c.Diag.Errored() {
		t.Fatalf("expected recursion-limit diagnostic, got none")
	}
	found := false
	for _, d := range c.Diag.Diagnostics() {
		if strings.Contains(d.Msg, "recursion limit") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no recursion-limit diagnostic in: %+v", c.Diag.Diagnostics())
	}
}

func TestSynthParamTargetExpands(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth Required on param P {
    emit on P {
        @meta("required", "true")
    }
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

func handler(@Required id: int) {
}
`)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["myApp"]
	var fn *ir.FuncDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if fd, ok := st.(*ir.FuncDeclStmt); ok && fd.Name.Name == "handler" {
				fn = fd
			}
		}
	}
	if fn == nil || len(fn.Params) != 1 {
		t.Fatalf("handler not found or wrong param count")
	}
	prm := fn.Params[0]
	if len(prm.Annotations) != 1 || prm.Annotations[0].Name.Name != "meta" {
		t.Fatalf("param id: want @meta after expansion, got %+v", prm.Annotations)
	}
}

func TestSynthMethodAndCtorTargetsExpand(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth Logged on method M {
    emit on M {
        @meta("logged", "yes")
    }
}

synth Tracked on ctor C {
    emit on C {
        @meta("tracked", "yes")
    }
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

type Service {
    @Tracked
    new(x: int) {
    }

    @Logged
    func handle() {
    }
}
`)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["myApp"]
	var td *ir.TypeDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if t, ok := st.(*ir.TypeDeclStmt); ok && t.Name.Name == "Service" {
				td = t
			}
		}
	}
	if td == nil {
		t.Fatalf("type Service missing")
	}
	if len(td.Methods) != 1 || len(td.Methods[0].Annotations) != 1 || td.Methods[0].Annotations[0].Name.Name != "meta" {
		t.Fatalf("method handle: want single @meta annotation, got %+v", td.Methods)
	}
	if len(td.Ctors) != 1 || len(td.Ctors[0].Annotations) != 1 || td.Ctors[0].Annotations[0].Name.Name != "meta" {
		t.Fatalf("ctor: want single @meta annotation, got %+v", td.Ctors)
	}
}

func TestSynthForLoopEmitsOnEveryField(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth GormModel(table: string) on type T {
    emit on T {
        @meta("table", table)
    }
    for f in T.fields {
        emit on f {
            @structTag("gorm", "column:" + f.name)
        }
    }
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

@GormModel("users")
type User {
    id: int
    name: string
    email: string
}
`)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["myApp"]
	var td *ir.TypeDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if t, ok := st.(*ir.TypeDeclStmt); ok && t.Name.Name == "User" {
				td = t
			}
		}
	}
	if td == nil {
		t.Fatalf("type User missing")
	}
	if len(td.Annotations) != 1 || td.Annotations[0].Name.Name != "meta" {
		t.Fatalf("User type annotations: want single @meta, got %+v", td.Annotations)
	}
	if td.Annotations[0].ResolvedArgs[1].Str != "users" {
		t.Fatalf("User table tag value: want 'users', got %q", td.Annotations[0].ResolvedArgs[1].Str)
	}
	wantPerField := map[string]string{
		"id":    "column:id",
		"name":  "column:name",
		"email": "column:email",
	}
	for _, fld := range td.Fields {
		want := wantPerField[fld.Name.Name]
		if len(fld.Annotations) != 1 {
			t.Fatalf("field %s: want 1 annotation, got %d", fld.Name.Name, len(fld.Annotations))
		}
		a := fld.Annotations[0]
		if a.Name.Name != "structTag" {
			t.Fatalf("field %s: want @structTag, got @%s", fld.Name.Name, a.Name.Name)
		}
		if a.ResolvedArgs[1].Str != want {
			t.Fatalf("field %s: want %q, got %q", fld.Name.Name, want, a.ResolvedArgs[1].Str)
		}
	}
}

func TestSynthForLoopWhereFiltersFields(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth PublicOnly on type T {
    for f in T.fields where f.isShared {
        emit on f {
            @meta("public", f.name)
        }
    }
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

@PublicOnly
type User {
    shared id: int = 0
    shared name: string = ""
    passwordHash: string = ""
}
`)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["myApp"]
	var td *ir.TypeDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if t, ok := st.(*ir.TypeDeclStmt); ok && t.Name.Name == "User" {
				td = t
			}
		}
	}
	if td == nil {
		t.Fatalf("type User missing")
	}
	counts := map[string]int{}
	for _, fld := range td.Fields {
		counts[fld.Name.Name] = len(fld.Annotations)
	}
	if counts["id"] != 1 {
		t.Fatalf("id should have @meta from where=isShared: got %d", counts["id"])
	}
	if counts["name"] != 1 {
		t.Fatalf("name should have @meta from where=isShared: got %d", counts["name"])
	}
	if counts["passwordHash"] != 0 {
		t.Fatalf("passwordHash should be unannotated (not shared): got %d", counts["passwordHash"])
	}
}

func TestSynthEmitFieldInjectsNewMember(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth Timestamps on type T {
    emit createdAt: int = 0
    emit updatedAt: int = 0
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

@Timestamps
type User {
    id: int
}
`)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["myApp"]
	var td *ir.TypeDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if t, ok := st.(*ir.TypeDeclStmt); ok && t.Name.Name == "User" {
				td = t
			}
		}
	}
	if td == nil {
		t.Fatalf("type User missing")
	}
	names := []string{}
	for _, fld := range td.Fields {
		names = append(names, fld.Name.Name)
	}
	want := []string{"id", "createdAt", "updatedAt"}
	if len(names) != len(want) {
		t.Fatalf("want fields %v, got %v", want, names)
	}
	for i, w := range want {
		if names[i] != w {
			t.Fatalf("field %d: want %q, got %q", i, w, names[i])
		}
	}
}

func TestSynthEmitMethodAndCtorInjectsNewMembers(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth WithCtorAndMethod on type T {
    emit new(seed: int) {
    }
    emit func ping(): int {
        return 1
    }
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

@WithCtorAndMethod
type Service {
    counter: int
}
`)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["myApp"]
	var td *ir.TypeDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if t, ok := st.(*ir.TypeDeclStmt); ok && t.Name.Name == "Service" {
				td = t
			}
		}
	}
	if td == nil {
		t.Fatalf("type Service missing")
	}
	if len(td.Methods) != 1 || td.Methods[0].Func == nil || td.Methods[0].Func.Name.Name != "ping" {
		t.Fatalf("want one method `ping`, got %+v", td.Methods)
	}
	if len(td.Ctors) == 0 {
		t.Fatalf("want at least one ctor, got none")
	}
	found := false
	for _, c := range td.Ctors {
		if len(c.Params) == 1 && c.Params[0].Name.Name == "seed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want ctor with `seed` param, got %+v", td.Ctors)
	}
}

func TestSynthEmitAppendToRegistry(t *testing.T) {
	c := New()
	c.AddSource("anno.sova", `package myAnno on synth

synth Route(method: string, path: string) on func G {
    emit on G {
        @meta("route", method + " " + path)
    }
    emit append to routes {
        path
    }
}
`)
	c.AddSource("model.sova", `package myApp on backend

import "myAnno"

@Route("GET", "/hello")
func hello() {
}

@Route("POST", "/world")
func world() {
}
`)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	reg, ok := c.Cache["synth_reg:routes"].([]ir.Expr)
	if !ok {
		t.Fatalf("synth_reg:routes missing from cache; got: %T", c.Cache["synth_reg:routes"])
	}
	if len(reg) != 2 {
		t.Fatalf("want 2 route entries, got %d", len(reg))
	}
	paths := make([]string, 0, len(reg))
	for _, e := range reg {
		if lit, ok := e.(*ir.LitString); ok {
			paths = append(paths, lit.Value)
		}
	}
	want1, want2 := "/hello", "/world"
	if !(paths[0] == want1 && paths[1] == want2 || paths[0] == want2 && paths[1] == want1) {
		t.Fatalf("registry paths: want {%q, %q} in either order, got %v", want1, want2, paths)
	}
}
