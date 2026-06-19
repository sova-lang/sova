package ir

import (
	"fmt"
	"strings"
)

type TypeKey string
type TypeKind int

const (
	TK_PrimitiveAny TypeKind = iota
	TK_PrimitiveNone
	TK_PrimitiveInt
	TK_PrimitiveFloat
	TK_PrimitiveBool
	TK_PrimitiveString
	TK_PrimitiveChar
	TK_PrimitiveByte
	TK_Option
	TK_Slice
	TK_Array
	TK_Map
	TK_Tuple
	TK_Function
	TK_Enum
	TK_Struct
	TK_Interface
	TK_TypeParam
	TK_Chan
)

type Type struct {
	ID   TypID
	Kind TypeKind
	Key  TypeKey

	Dim int64

	ElemType TypID

	KeyType   TypID
	ValueType TypID

	Fields []TupleField

	ParamTypes []*FuncParam
	ReturnType TypID
	IsAsync    bool

	EnumName    string
	EnumCases   []EnumCaseInfo
	EnumFields  []EnumFieldInfo
	EnumMethods []EnumMethodInfo
	IsNumeric   bool

	PackagePath string

	StructName       string
	StructFields     []StructFieldInfo
	StructCtors      []StructCtorInfo
	StructMethods    []StructMethodInfo
	StructImplements []TypID
	StructCasts      []StructCastInfo

	InterfaceName    string
	InterfaceMethods []InterfaceSigInfo

	IsExtern     bool
	ExternModule string
	ExternSide   SideKind
	ExternValue  bool

	IsComposable bool

	ParamName  string
	TypeParams []string
}

type InterfaceSigInfo struct {
	Name     string
	FuncTyp  TypID
	IsShared bool
}

type StructMethodInfo struct {
	Name               string
	Sym                SymID
	FuncTyp            TypID
	IsPromoted         bool
	PromotedFromExtern bool
	IsShared           bool
}

type StructFieldInfo struct {
	Name               string
	Type               TypID
	Private            bool
	Sym                SymID
	IsPromoted         bool
	PromotedFromExtern bool
	IsReactive         bool
	IsShared           bool
}

type StructCtorInfo struct {
	Sym     SymID
	FuncTyp TypID
}

type StructCastInfo struct {
	Sym       SymID
	SourceTyp TypID
	FuncTyp   TypID
}

type EnumCaseInfo struct {
	Name    string
	Ordinal int
	Value   int64
}

type EnumFieldInfo struct {
	Name string
	Type TypID
}

type EnumMethodInfo struct {
	Name string
	Type TypID
}

func PrimitiveType(kind TypeKind) *Type {
	ty := &Type{
		ID:   0,
		Kind: kind,
	}

	ty.Key = generateTypeKey(ty)
	return ty
}

func OptionType(elemType TypID) *Type {
	ty := &Type{
		ID:       0,
		Kind:     TK_Option,
		ElemType: elemType,
	}

	ty.Key = generateTypeKey(ty)
	return ty
}

func ArrayType(elemType TypID, dim int64) *Type {
	ty := &Type{
		ID:       0,
		Kind:     TK_Array,
		ElemType: elemType,
		Dim:      dim,
	}

	ty.Key = generateTypeKey(ty)
	return ty
}

func SliceType(elemType TypID) *Type {
	ty := &Type{
		ID:       0,
		Kind:     TK_Slice,
		ElemType: elemType,
	}

	ty.Key = generateTypeKey(ty)
	return ty
}

func MapType(keyType, valueType TypID) *Type {
	ty := &Type{
		ID:        0,
		Kind:      TK_Map,
		KeyType:   keyType,
		ValueType: valueType,
	}

	ty.Key = generateTypeKey(ty)
	return ty
}

func TupleType(fields ...TupleField) *Type {
	ty := &Type{
		ID:     0,
		Kind:   TK_Tuple,
		Fields: fields,
	}

	ty.Key = generateTypeKey(ty)
	return ty
}

type TupleField struct {
	Name string
	Type TypID
}

func FuncType(paramTypes []*FuncParam, returnType TypID) *Type {
	ty := &Type{
		ID:         0,
		Kind:       TK_Function,
		ParamTypes: paramTypes,
		ReturnType: returnType,
	}

	ty.Key = generateTypeKey(ty)
	return ty
}

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

func EnumType(name string, cases []EnumCaseInfo, fields []EnumFieldInfo, isNumeric bool) *Type {
	ty := &Type{
		ID:         0,
		Kind:       TK_Enum,
		EnumName:   name,
		EnumCases:  cases,
		EnumFields: fields,
		IsNumeric:  isNumeric,
	}

	ty.Key = generateTypeKey(ty)
	return ty
}

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

type TypeTable struct {
	types      map[TypeKey]TypID
	byID       map[TypID]*Type
	alloc      *IdAlloc
	primAny    TypID
	primInt    TypID
	primFloat  TypID
	primString TypID
	primBool   TypID
	primChar   TypID
	primByte   TypID
	typError   TypID
	typNone    TypID
}

func NewTypeTable(alloc *IdAlloc) *TypeTable {
	tbl := &TypeTable{
		types: make(map[TypeKey]TypID),
		byID:  make(map[TypID]*Type),
		alloc: alloc,
	}

	tbl.primAny = tbl.DeclareType(PrimitiveType(TK_PrimitiveAny))
	tbl.primInt = tbl.DeclareType(PrimitiveType(TK_PrimitiveInt))
	tbl.primFloat = tbl.DeclareType(PrimitiveType(TK_PrimitiveFloat))
	tbl.primString = tbl.DeclareType(PrimitiveType(TK_PrimitiveString))
	tbl.primBool = tbl.DeclareType(PrimitiveType(TK_PrimitiveBool))
	tbl.primChar = tbl.DeclareType(PrimitiveType(TK_PrimitiveChar))
	tbl.primByte = tbl.DeclareType(PrimitiveType(TK_PrimitiveByte))
	tbl.typError = tbl.DeclareType(&Type{Key: "!internal_error"})
	tbl.typNone = tbl.DeclareType(PrimitiveType(TK_PrimitiveNone))

	return tbl
}

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

func (t *TypeTable) GetByID(id TypID) (*Type, bool) {
	typ, exists := t.byID[id]
	return typ, exists
}

func (t *TypeTable) All() []*Type {
	out := make([]*Type, 0, len(t.byID))
	for _, ty := range t.byID {
		out = append(out, ty)
	}

	return out
}

func (t *TypeTable) GetByType(key TypeKey) (TypID, bool) {
	id, exists := t.types[key]
	return id, exists
}

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

func (t *TypeTable) PrimByte() TypID {
	return t.primByte
}

func (t *TypeTable) TypError() TypID {
	return t.typError
}

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

func (t *TypeTable) AsyncFuncOf(paramTypes []*FuncParam, returnType TypID) TypID {
	typ := AsyncFuncType(paramTypes, returnType)
	return t.DeclareType(typ)
}

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

func (t *TypeTable) StructOf(pkgPath, name string, fields []StructFieldInfo) TypID {
	typ := StructType(name, fields)
	typ.PackagePath = pkgPath
	typ.Key = generateTypeKey(typ)
	return t.DeclareType(typ)
}

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

func (t *TypeTable) HasAnyOfKind(kind TypeKind) bool {
	for _, ty := range t.byID {
		if ty.Kind == kind {
			return true
		}
	}

	return false
}

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
