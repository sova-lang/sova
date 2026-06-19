package ir

type TypeRef struct {
	node
	Kind TypeKind
	Typ  TypID

	Elem *TypeRef
	Dim  int64

	Key   *TypeRef
	Value *TypeRef

	Tuple []TupleFieldRef

	FuncParams []FuncTypeParamRef
	FuncReturn *TypeRef

	CustomName      string
	CustomQualifier string
	TypeArgs        []*TypeRef
}

type FuncTypeParamRef struct {
	Name string
	Type *TypeRef
}

type TupleFieldRef struct {
	Name string
	Type *TypeRef
}
