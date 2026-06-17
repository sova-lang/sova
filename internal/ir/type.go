package ir

import (
	"fmt"
	"strings"
)

type TypeKey string // A unique type key which is same for all instances of a type.
type TypeKind int   // TypeKind represents the kind of a type in the intermediate representation.

const (
	TK_PrimitiveAny    TypeKind = iota // TK_PrimitiveAny represents a primitive any type.
	TK_PrimitiveNone                   // TK_PrimitiveNone represents a primitive none type.
	TK_PrimitiveInt                    // TK_PrimitiveInt represents a primitive integer type.
	TK_PrimitiveFloat                  // TK_PrimitiveFloat represents a primitive floating-point type.
	TK_PrimitiveBool                   // TK_PrimitiveBool represents a primitive boolean type.
	TK_PrimitiveString                 // TK_PrimitiveString represents a primitive string type.
	TK_PrimitiveChar                   // TK_PrimitiveChar represents a primitive character type.
	TK_PrimitiveByte                   // TK_PrimitiveByte represents a primitive byte (uint8, 0..255) type.
	TK_Option                          // TK_Option represents an option type (none or some).
	TK_Slice                           // TK_Slice represents a slice type.
	TK_Array                           // TK_Array represents an array type.
	TK_Map                             // TK_Map represents a map type.
	TK_Tuple                           // TK_Tuple represents a tuple type.
	TK_Function                        // TK_Function represents a function type.
	TK_Enum                            // TK_Enum represents an enumeration type.
	TK_Struct                          // TK_Struct represents a user-defined record type.
	TK_Interface                       // TK_Interface represents a behavioral contract type.
	TK_TypeParam                       // TK_TypeParam represents a generic type parameter (e.g. `T` inside `type List<T> { ... }`). Opaque to the type system; the codegen emits the parameter name directly.
	TK_Chan                            // TK_Chan represents a typed channel value (`chan<T>`). Backend lowers to Go's `chan T`; frontend lowers to a runtime-shim async queue. Element type stored in ElemType.
)

// Type represents a type in the intermediate representation. A type is a classification that specifies the nature of a value.
type Type struct {
	ID   TypID    // ID is the unique identifier for the type.
	Kind TypeKind // Kind represents the kind of the type.
	Key  TypeKey  // Key is a unique key for the type, used for type equality checks.

	// Array/Slice related:
	Dim int64 // Dim defines the depth of the slice or the size of the array.

	// Array/slice/option related:
	ElemType TypID // ElemType is the element type for array/slice/option types.

	// (Unordered-)Map related:
	KeyType   TypID // KeyType is the key type for map types.
	ValueType TypID // ValueType is the value type for map types.

	// Tuple related:
	Fields []TupleField // Fields represents the fields in a tuple type.

	// Function related:
	ParamTypes []*FuncParam // ParamTypes represents the parameter types of a function type.
	ReturnType TypID        // ReturnType represents the return type of a function type. If the function returns multiple values, this is a tuple type.
	IsAsync    bool         // IsAsync is true when the function type belongs to an async function.

	// Enum related:
	EnumName    string           // EnumName is the name of the enum (for TK_Enum).
	EnumCases   []EnumCaseInfo   // EnumCases stores resolved info about each case.
	EnumFields  []EnumFieldInfo  // EnumFields stores payload field definitions.
	EnumMethods []EnumMethodInfo // EnumMethods stores method definitions.
	IsNumeric   bool             // IsNumeric is true for numeric enums, false for payload enums.

	// User-type package path: set for TK_Struct / TK_Enum / TK_Interface so that the same name in different packages produces distinct keys (and thus distinct TypIDs). Empty string means "no package" - used for type-universe-global types like the synthetic Session / Broadcast / WireState that are deliberately shared across all imports.
	PackagePath string

	// Struct related:
	StructName       string             // StructName is the user-facing name of the struct (for TK_Struct).
	StructFields     []StructFieldInfo  // StructFields stores resolved field definitions in declaration order.
	StructCtors      []StructCtorInfo   // StructCtors stores resolved info about each explicit constructor.
	StructMethods    []StructMethodInfo // StructMethods stores resolved info about each declared method.
	StructImplements []TypID            // StructImplements stores resolved interface type ids the struct implements.
	StructCasts      []StructCastInfo   // StructCasts stores cast-from declarations: each entry maps a source TypID to a cast function sym that produces this struct type.

	// Interface related:
	InterfaceName    string             // InterfaceName is the user-facing name of the interface (for TK_Interface).
	InterfaceMethods []InterfaceSigInfo // InterfaceMethods lists method signatures the interface mandates.

	// Extern interop related: when true the type is a binding to a host-language definition. Module is the host import path (Go) or module specifier (JS); empty for non-extern types. ExternSide is the side of the file the extern was declared in; used to reject references from the wrong side. ExternValue is true for native value-types (e.g. Go's `time.Time`) and suppresses the default `*` pointer prefix in Go codegen; set via the `@value` annotation on the extern declaration.
	IsExtern     bool
	ExternModule string
	ExternSide   SideKind
	ExternValue  bool

	// IsComposable is true when the type's `with`-chain (after mixin resolution) eventually reaches the built-in `Composable` mixin. Set by `analyze_composables` and consumed by composable-block validation + codegen.
	IsComposable bool

	// TypeParam-related: for TK_TypeParam types, ParamName is the user-written name (e.g. "T"). For TK_Struct, TypeParams lists the declared parameter names so the codegen can emit `[T any, U any, ...]` after the struct/func name.
	ParamName  string
	TypeParams []string
}

// InterfaceSigInfo describes one method signature required by an interface.
type InterfaceSigInfo struct {
	Name     string // Name is the method name in the interface.
	FuncTyp  TypID  // FuncTyp is the resolved function type of the signature.
	IsShared bool   // IsShared is true when the contract requires implementations to mark their matching method `shared` so the body emits on both sides. Set explicitly by the `shared` modifier or implicitly when the interface's declaring file is `on shared`.
}

// StructMethodInfo stores resolved info about a single declared method of a struct.
type StructMethodInfo struct {
	Name               string // Name is the user-facing method name.
	Sym                SymID  // Sym is the method function symbol.
	FuncTyp            TypID  // FuncTyp is the function type of the method.
	IsPromoted         bool   // IsPromoted is true when the method was lifted from an embedded type and is not declared on the struct directly.
	PromotedFromExtern bool   // PromotedFromExtern is true when the source type of the promotion was an extern binding (Go-side name = Sova-side name, no PascalCase coercion).
	IsShared           bool   // IsShared mirrors the `shared` modifier on the declaring TypeMethodDecl. The interface-conformance check consults it: when an interface method is `IsShared`, every implementing type's matching method must also be `IsShared`.
}

// StructFieldInfo stores resolved info about a struct field.
type StructFieldInfo struct {
	Name               string // Name is the field name.
	Type               TypID  // Type is the field type.
	Private            bool   // Private is true when the field is restricted to the declaring type.
	Sym                SymID  // Sym is the field symbol id within the type's scope.
	IsPromoted         bool   // IsPromoted is true when the field was lifted from an embedded type and is not declared on the struct directly.
	PromotedFromExtern bool   // PromotedFromExtern is true when the source type of the promotion was an extern binding.
	IsReactive         bool   // IsReactive is true when the field carries the `@reactive` annotation; codegen generates an observer list, setter, and `observe<Field>` method.
	IsShared           bool   // IsShared mirrors the `shared` modifier on the declaring TypeField. Used by the LSP to filter cross-side member completion: a frontend file accessing a backend-declared type only sees fields whose IsShared is true.
}

// StructCtorInfo stores resolved info about a single explicit constructor of a struct.
type StructCtorInfo struct {
	Sym     SymID // Sym is the constructor function symbol.
	FuncTyp TypID // FuncTyp is the function type describing the constructor signature.
}

// StructCastInfo stores resolved info about a single `cast(p: SourceT): Self` declaration. The compiler may auto-insert a call to Sym wherever a value of SourceTyp appears where this struct's type is expected.
type StructCastInfo struct {
	Sym       SymID // Sym is the cast function symbol.
	SourceTyp TypID // SourceTyp is the type the cast accepts.
	FuncTyp   TypID // FuncTyp is the resolved function type (SourceTyp) -> Self.
}

// EnumCaseInfo stores resolved info about an enum case.
type EnumCaseInfo struct {
	Name    string // Name is the case name.
	Ordinal int    // Ordinal is the 0-indexed position.
	Value   int64  // Value is the integer value (for numeric enums).
}

// EnumFieldInfo stores info about a payload enum field.
type EnumFieldInfo struct {
	Name string // Name is the field name.
	Type TypID  // Type is the field type.
}

// EnumMethodInfo stores info about an enum method.
type EnumMethodInfo struct {
	Name string // Name is the method name.
	Type TypID  // Type is the function type of the method.
}

// PrimitiveType returns a primitive type with the given kind.
func PrimitiveType(kind TypeKind) *Type {
	ty := &Type{
		ID:   0, // ID will be set later.
		Kind: kind,
	}
	ty.Key = generateTypeKey(ty)
	return ty
}

// OptionType returns an option type with the given element type.
func OptionType(elemType TypID) *Type {
	ty := &Type{
		ID:       0, // ID will be set later.
		Kind:     TK_Option,
		ElemType: elemType,
	}
	ty.Key = generateTypeKey(ty)
	return ty
}

// ArrayType returns a slice type with the given element type and dimensions.
func ArrayType(elemType TypID, dim int64) *Type {
	ty := &Type{
		ID:       0, // ID will be set later.
		Kind:     TK_Array,
		ElemType: elemType,
		Dim:      dim,
	}
	ty.Key = generateTypeKey(ty)
	return ty
}

// SliceType returns an array type with the given element type.
func SliceType(elemType TypID) *Type {
	ty := &Type{
		ID:       0, // ID will be set later.
		Kind:     TK_Slice,
		ElemType: elemType,
	}
	ty.Key = generateTypeKey(ty)
	return ty
}

// MapType returns a map type with the given key and value types.
func MapType(keyType, valueType TypID) *Type {
	ty := &Type{
		ID:        0, // ID will be set later.
		Kind:      TK_Map,
		KeyType:   keyType,
		ValueType: valueType,
	}
	ty.Key = generateTypeKey(ty)
	return ty
}

// TupleType returns a tuple type with the given fields.
func TupleType(fields ...TupleField) *Type {
	ty := &Type{
		ID:     0, // ID will be set later.
		Kind:   TK_Tuple,
		Fields: fields,
	}
	ty.Key = generateTypeKey(ty)
	return ty
}

// TupleField is a field within a type in the intermediate representation used for tuples.
type TupleField struct {
	Name string // Name is the name of the field. Can be empty, if the field is unnamed.
	Type TypID  // Type is the type of the field.
}

// FuncType returns a function type with the given parameter and return types.
func FuncType(paramTypes []*FuncParam, returnType TypID) *Type {
	ty := &Type{
		ID:         0, // ID will be set later.
		Kind:       TK_Function,
		ParamTypes: paramTypes,
		ReturnType: returnType,
	}
	ty.Key = generateTypeKey(ty)
	return ty
}

// AsyncFuncType returns a function type marked as asynchronous.
func AsyncFuncType(paramTypes []*FuncParam, returnType TypID) *Type {
	ty := &Type{
		ID:         0,
		Kind:       TK_Function,
		ParamTypes: paramTypes,
		ReturnType: returnType,
		IsAsync:    true,
	}
	ty.Key = generateTypeKey(ty)
	return ty
}

// EnumType returns an enum type with the given name, cases, fields, and numeric flag.
func EnumType(name string, cases []EnumCaseInfo, fields []EnumFieldInfo, isNumeric bool) *Type {
	ty := &Type{
		ID:         0, // ID will be set later.
		Kind:       TK_Enum,
		EnumName:   name,
		EnumCases:  cases,
		EnumFields: fields,
		IsNumeric:  isNumeric,
	}
	ty.Key = generateTypeKey(ty)
	return ty
}

// StructType returns a struct type carrying the user-facing name and resolved fields.
func StructType(name string, fields []StructFieldInfo) *Type {
	ty := &Type{
		ID:           0,
		Kind:         TK_Struct,
		StructName:   name,
		StructFields: fields,
	}
	ty.Key = generateTypeKey(ty)
	return ty
}

// TypeArena is a table mapping type IDs to types. It is used to store all types in the intermediate representation.
type TypeTable struct {
	types      map[TypeKey]TypID // types maps type keys to type IDs, allowing for unique identification of types.
	byID       map[TypID]*Type   // byID maps type IDs to types.
	alloc      *IdAlloc          // alloc is used to allocate new type IDs.
	primAny    TypID             // primAny is the ID of the primitive any type.
	primInt    TypID             // primInt is the ID of the primitive integer type.
	primFloat  TypID             // primFloat is the ID of the primitive float type.
	primString TypID             // primString is the ID of the primitive string type.
	primBool   TypID             // primBool is the ID of the primitive boolean type.
	primChar   TypID             // primChar is the ID of the primitive character type.
	primByte   TypID             // primByte is the ID of the primitive byte (uint8) type.
	typError   TypID             // typError is the ID of the error type, used for error handling in the IR.
	typNone    TypID             // typNone is the ID of the none type, used for option types.
}

// NewTypeTable returns a new instance of TypeTable.
func NewTypeTable(alloc *IdAlloc) *TypeTable {
	tbl := &TypeTable{
		types: make(map[TypeKey]TypID),
		byID:  make(map[TypID]*Type),
		alloc: alloc,
	}

	tbl.primAny = tbl.DeclareType(PrimitiveType(TK_PrimitiveAny))       // Primitive any type.
	tbl.primInt = tbl.DeclareType(PrimitiveType(TK_PrimitiveInt))       // Primitive integer type.
	tbl.primFloat = tbl.DeclareType(PrimitiveType(TK_PrimitiveFloat))   // Primitive float type.
	tbl.primString = tbl.DeclareType(PrimitiveType(TK_PrimitiveString)) // Primitive string type.
	tbl.primBool = tbl.DeclareType(PrimitiveType(TK_PrimitiveBool))     // Primitive boolean type.
	tbl.primChar = tbl.DeclareType(PrimitiveType(TK_PrimitiveChar))     // Primitive character type.
	tbl.primByte = tbl.DeclareType(PrimitiveType(TK_PrimitiveByte))     // Primitive byte (uint8) type.
	tbl.typError = tbl.DeclareType(&Type{Key: "!internal_error"})
	tbl.typNone = tbl.DeclareType(PrimitiveType(TK_PrimitiveNone)) // None type for option types.

	return tbl
}

// DeclareType registers a new type in the type table and returns its ID.
// Remark: The compiler uses a type universe meaning that all types are global per build. Types in one package should be prefixed with their package name to avoid conflicts.
func (t *TypeTable) DeclareType(typ *Type) TypID {
	if _, exists := t.types[typ.Key]; exists {
		return t.types[typ.Key]
	}
	id := TypID(t.alloc.Next())
	typ.ID = id
	t.byID[id] = typ
	t.types[typ.Key] = id
	return id
}

// GetByID returns the type associated with the given ID, if it exists.
func (t *TypeTable) GetByID(id TypID) (*Type, bool) {
	typ, exists := t.byID[id]
	return typ, exists
}

// All returns a snapshot of every registered type. Callers should treat the result as
// read-only-ish: mutating fields on the returned `*Type` pointers IS the way passes update
// the universe (e.g. propagate_async refreshes cached method async-ness here), but the slice
// itself is a copy so concurrent registrations during iteration don't disturb the walk.
func (t *TypeTable) All() []*Type {
	out := make([]*Type, 0, len(t.byID))
	for _, ty := range t.byID {
		out = append(out, ty)
	}
	return out
}

// GetByType returns the ID associated with the given type key, if it exists.
func (t *TypeTable) GetByType(key TypeKey) (TypID, bool) {
	id, exists := t.types[key]
	return id, exists
}

// RegisterAlias records that `alias:<pkg>:<name>` points to the same TypID as the aliased type. Type-name lookups consult this same map so users of `MyAlias` (or `pkg.MyAlias`) resolve to the underlying type transparently.
func (t *TypeTable) RegisterAlias(pkgPath, name string, target TypID) {
	t.types[TypeKey("alias:"+pkgPath+":"+name)] = target
}

func (t *TypeTable) PrimAny() TypID {
	return t.primAny
}

func (t *TypeTable) PrimInt() TypID {
	return t.primInt
}

func (t *TypeTable) PrimFloat() TypID {
	return t.primFloat
}

func (t *TypeTable) PrimString() TypID {
	return t.primString
}

func (t *TypeTable) PrimBool() TypID {
	return t.primBool
}

func (t *TypeTable) PrimChar() TypID {
	return t.primChar
}

// PrimByte returns the ID of the primitive byte (uint8, range 0..255) type.
func (t *TypeTable) PrimByte() TypID {
	return t.primByte
}

// TypError returns the ID of the error type, used for error handling in the IR (e.g. invalid types).
func (t *TypeTable) TypError() TypID {
	return t.typError
}

// TypNone returns the ID of the none type, used for option types (e.g. Option<T>).
func (t *TypeTable) TypNone() TypID {
	return t.typNone
}

func (t *TypeTable) ArrayOf(elemType TypID, dim int64) TypID {
	typ := ArrayType(elemType, dim)
	return t.DeclareType(typ)
}

func (t *TypeTable) SliceOf(elemType TypID) TypID {
	typ := SliceType(elemType)
	return t.DeclareType(typ)
}

func (t *TypeTable) OptionOf(elemType TypID) TypID {
	typ := OptionType(elemType)
	return t.DeclareType(typ)
}

func (t *TypeTable) MapOf(keyType, valueType TypID) TypID {
	typ := MapType(keyType, valueType)
	return t.DeclareType(typ)
}

func (t *TypeTable) TupleOf(fields ...TupleField) TypID {
	typ := TupleType(fields...)
	return t.DeclareType(typ)
}

func (t *TypeTable) FuncOf(paramTypes []*FuncParam, returnType TypID) TypID {
	typ := FuncType(paramTypes, returnType)
	return t.DeclareType(typ)
}

// AsyncFuncOf registers an async function type with the given parameter and return types.
func (t *TypeTable) AsyncFuncOf(paramTypes []*FuncParam, returnType TypID) TypID {
	typ := AsyncFuncType(paramTypes, returnType)
	return t.DeclareType(typ)
}

// ChanOf registers a channel-type whose element type is `elemType`. Idempotent: returns the existing TypID if the same channel type was already registered.
func (t *TypeTable) ChanOf(elemType TypID) TypID {
	key := TypeKey(fmt.Sprintf("chan:%d", elemType))
	if id, ok := t.GetByType(key); ok {
		return id
	}
	typ := &Type{
		Kind:     TK_Chan,
		Key:      key,
		ElemType: elemType,
	}
	return t.DeclareType(typ)
}

// TypeParamOf registers a generic type-parameter type with the given user-written name and an owner-scope key that disambiguates `T` in `List<T>` from `T` in `Map<K, T>`. The owner key is the joined-package-and-decl path so identical parameter names declared in different generic decls produce distinct TypIDs.
func (t *TypeTable) TypeParamOf(ownerKey, name string) TypID {
	key := TypeKey("typeparam:" + ownerKey + ":" + name)
	if id, ok := t.GetByType(key); ok {
		return id
	}
	typ := &Type{
		Kind:      TK_TypeParam,
		Key:       key,
		ParamName: name,
	}
	return t.DeclareType(typ)
}

func (t *TypeTable) EnumOf(pkgPath, name string, cases []EnumCaseInfo, fields []EnumFieldInfo, isNumeric bool) TypID {
	typ := EnumType(name, cases, fields, isNumeric)
	typ.PackagePath = pkgPath
	typ.Key = generateTypeKey(typ)
	return t.DeclareType(typ)
}

// StructOf registers a struct type with the given user-facing name and resolved fields. `pkgPath` is the importable path of the declaring package (e.g. "app/models"), empty for compiler-internal "global" types like Session/Broadcast/error that are visible everywhere by design.
func (t *TypeTable) StructOf(pkgPath, name string, fields []StructFieldInfo) TypID {
	typ := StructType(name, fields)
	typ.PackagePath = pkgPath
	typ.Key = generateTypeKey(typ)
	return t.DeclareType(typ)
}

// InterfaceOf registers an interface type with the given user-facing name. `pkgPath` is the importable path of the declaring package, empty for compiler-internal globals.
func (t *TypeTable) InterfaceOf(pkgPath, name string) TypID {
	typ := &Type{Kind: TK_Interface, InterfaceName: name, PackagePath: pkgPath}
	typ.Key = generateTypeKey(typ)
	return t.DeclareType(typ)
}

func (t *TypeTable) IsTypeOfKind(typ TypID, base TypeKind) bool {
	actualType, ok := t.GetByID(typ)
	if !ok {
		return false
	}
	return actualType.Kind == base
}

// HasAnyOfKind reports whether the type table currently holds any type whose Kind matches `kind`. Useful for codegen pre-walks that want to inject runtime shims only when the program actually uses a given construct (e.g. channels).
func (t *TypeTable) HasAnyOfKind(kind TypeKind) bool {
	for _, ty := range t.byID {
		if ty.Kind == kind {
			return true
		}
	}
	return false
}

// GetFunctionSignatureKey returns a string key representing the function signature for overload resolution.
// This includes parameter types but not the return type, as overloading is based on parameters only.
func (t *TypeTable) GetFunctionSignatureKey(funcTyp TypID) string {
	typ, ok := t.GetByID(funcTyp)
	if !ok || typ.Kind != TK_Function {
		return ""
	}

	params := make([]string, len(typ.ParamTypes))
	for i, param := range typ.ParamTypes {
		params[i] = fmt.Sprintf("%d", param.Type.Typ)
	}
	return strings.Join(params, ",")
}

func generateTypeKey(typ *Type) TypeKey {
	switch typ.Kind {
	case TK_PrimitiveAny:
		return "any"
	case TK_PrimitiveNone:
		return "none"
	case TK_PrimitiveInt:
		return "int"
	case TK_PrimitiveFloat:
		return "float"
	case TK_PrimitiveString:
		return "string"
	case TK_PrimitiveBool:
		return "bool"
	case TK_PrimitiveChar:
		return "char"
	case TK_PrimitiveByte:
		return "byte"
	case TK_Option:
		return TypeKey(fmt.Sprintf("option:%d", typ.ElemType))
	case TK_Array:
		return TypeKey(fmt.Sprintf("array:%d:%v", typ.ElemType, typ.Dim))
	case TK_Slice:
		return TypeKey(fmt.Sprintf("slice:%d", typ.ElemType))
	case TK_Map:
		return TypeKey(fmt.Sprintf("map:%d:%d", typ.KeyType, typ.ValueType))
	case TK_Tuple:
		fields := make([]string, len(typ.Fields))
		for i, field := range typ.Fields {
			fields[i] = fmt.Sprintf("%s:%d", field.Name, field.Type)
		}
		return TypeKey(fmt.Sprintf("tuple:%s", strings.Join(fields, ",")))
	case TK_Function:
		params := make([]string, len(typ.ParamTypes))
		for i, param := range typ.ParamTypes {
			variadicStr := ""
			if param.IsVariadic {
				variadicStr = "..."
			}

			params[i] = fmt.Sprintf("%s%d", variadicStr, param.Type.Typ)
		}
		asyncPrefix := ""
		if typ.IsAsync {
			asyncPrefix = "async:"
		}
		return TypeKey(fmt.Sprintf("%sfunc:(%s)->%d", asyncPrefix, strings.Join(params, ","), typ.ReturnType))
	case TK_Enum:
		return TypeKey(fmt.Sprintf("enum:%s:%s", typ.PackagePath, typ.EnumName))
	case TK_Struct:
		return TypeKey(fmt.Sprintf("struct:%s:%s", typ.PackagePath, typ.StructName))
	case TK_Interface:
		return TypeKey(fmt.Sprintf("interface:%s:%s", typ.PackagePath, typ.InterfaceName))
	default:
		panic(fmt.Sprintf("unknown type kind: %d", typ.Kind))
	}
}
