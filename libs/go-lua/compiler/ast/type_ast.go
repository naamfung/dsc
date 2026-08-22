package ast

// TypeExpr represents a type annotation in the AST.
// Type expressions are used for type annotations, type declarations,
// and type-related operations in the typed Lua extension.
type TypeExpr interface {
	PositionHolder
	typeExprMarker()
}

// TypeExprBase provides the base implementation for all type expressions.
type TypeExprBase struct {
	Node
}

func (t *TypeExprBase) typeExprMarker() {}

// AnnotationExpr represents a type annotation like @min(0) or @pattern("^.+$")
type AnnotationExpr struct {
	Name string // "min", "max", "pattern", etc.
	Args []Expr // literal argument values
}

// PrimitiveTypeExpr represents primitive types: number, string, boolean, nil, any, unknown, never, integer
type PrimitiveTypeExpr struct {
	TypeExprBase
	Name        string           // "number", "string", "boolean", "nil", "any", "unknown", "never", "integer"
	Annotations []AnnotationExpr // runtime validation annotations
}

// OptionalTypeExpr represents an optional type: T?
type OptionalTypeExpr struct {
	TypeExprBase
	Inner TypeExpr
}

// UnionTypeExpr represents a union type: A | B | C
type UnionTypeExpr struct {
	TypeExprBase
	Types []TypeExpr
}

// IntersectionTypeExpr represents an intersection type: A & B
type IntersectionTypeExpr struct {
	TypeExprBase
	Types []TypeExpr
}

// ArrayTypeExpr represents an array type: {T}
type ArrayTypeExpr struct {
	TypeExprBase
	Element            TypeExpr
	Readonly           bool
	ElementAnnotations []AnnotationExpr // annotations on element type (before [])
	ArrayAnnotations   []AnnotationExpr // annotations on array itself (after [])
}

// MapTypeExpr represents a map type: {K: V}
type MapTypeExpr struct {
	TypeExprBase
	Key      TypeExpr
	Value    TypeExpr
	Readonly bool
}

// RecordFieldExpr represents a field in a record type.
type RecordFieldExpr struct {
	Name        string
	Type        TypeExpr
	Optional    bool
	Annotations []AnnotationExpr // runtime validation annotations on field
}

// RecordTypeExpr represents a record/table type: {name: string, age: number}
type RecordTypeExpr struct {
	TypeExprBase
	Fields   []RecordFieldExpr
	Readonly bool
}

// TypeParamExpr represents a type parameter in generic types.
type TypeParamExpr struct {
	Name       string
	Constraint TypeExpr // nil = any
}

// FunctionParamExpr represents a function parameter with optional name and type.
type FunctionParamExpr struct {
	Name string   // parameter name (may be empty for anonymous)
	Type TypeExpr // parameter type
}

// FunctionTypeExpr represents a function type: (A, B) -> (C, D)
type FunctionTypeExpr struct {
	TypeExprBase
	TypeParams []TypeParamExpr
	Params     []FunctionParamExpr
	Variadic   TypeExpr // nil if not variadic
	Returns    []TypeExpr
}

// AssertsTypeExpr represents a type assertion in return position: asserts x is T
type AssertsTypeExpr struct {
	TypeExprBase
	ParamName string   // the parameter being asserted
	NarrowTo  TypeExpr // the type to narrow to (nil means truthy/non-nil)
}

// TypeRefExpr represents a type reference: User, http.Request
type TypeRefExpr struct {
	TypeExprBase
	Path []string // ["User"] or ["http", "Request"]
}

// GenericTypeExpr represents an instantiated generic type: Array<T>, Map<K, V>
type GenericTypeExpr struct {
	TypeExprBase
	Base *TypeRefExpr
	Args []TypeExpr
}

// LiteralTypeExpr represents a literal type: "red" | 42 | true
type LiteralTypeExpr struct {
	TypeExprBase
	Value interface{} // string, float64, bool
}

// MetaTypeExpr represents a metatype: type<User>
type MetaTypeExpr struct {
	TypeExprBase
	Inner TypeExpr
}

// SelfTypeExpr represents the self type in method declarations.
type SelfTypeExpr struct {
	TypeExprBase
}

// TupleTypeExpr represents a tuple type: (A, B, C)
type TupleTypeExpr struct {
	TypeExprBase
	Elements []TypeExpr
}

// TypeOfExpr represents typeof(expr) - captures the inferred type of an expression.
//
// Unlike TypeScript which restricts typeof to identifiers, we follow Luau's approach
// allowing any expression. This enables patterns like:
//
//	type Config = typeof(defaultConfig)           -- capture variable type
//	type Person = typeof({ name = "", age = 0 })  -- capture table literal type
//	type API = typeof(require("api"))             -- capture module type
//
// During type checking, the Expr is synthesized and its type becomes the result.
type TypeOfExpr struct {
	TypeExprBase
	Expr Expr // the expression whose type to capture
}

// KeyOfExpr represents keyof T - extracts record keys as a union of string literals.
//
//	type Person = {name: string, age: number}
//	type Keys = keyof Person  -- "name" | "age"
//
// For non-record types, keyof returns never.
type KeyOfExpr struct {
	TypeExprBase
	Inner TypeExpr
}

// IndexAccessExpr represents T[K] - extracts the type of a field by key.
//
//	type Person = {name: string, age: number}
//	type NameType = Person["name"]  -- string
//
// Supports records, tuples (numeric index), and arrays.
type IndexAccessExpr struct {
	TypeExprBase
	Object TypeExpr // the type being indexed
	Index  TypeExpr // the key (usually a literal type)
}

// ConditionalTypeExpr represents T extends U ? A : B - type-level conditional.
//
//	type IsString<T> = T extends string ? true : false
//
// When T is a union, the conditional distributes over each member.
type ConditionalTypeExpr struct {
	TypeExprBase
	Check   TypeExpr // the type to check
	Extends TypeExpr // the constraint type
	Then    TypeExpr // result if Check extends Extends
	Else    TypeExpr // result if not
}

// TypeDefStmt represents a type alias declaration: type User = {name: string}
type TypeDefStmt struct {
	StmtBase
	Name       string
	TypeParams []TypeParamExpr
	Type       TypeExpr
}

// InterfaceMethodExpr represents a method signature in an interface.
type InterfaceMethodExpr struct {
	Name string
	Type *FunctionTypeExpr
}

// InterfaceDefStmt represents an interface declaration: interface Serializable ... end
type InterfaceDefStmt struct {
	StmtBase
	Name    string
	Extends []*TypeRefExpr
	Fields  []RecordFieldExpr
	Methods []InterfaceMethodExpr
}

// CastExpr represents an unsafe type cast: expr :: Type
type CastExpr struct {
	ExprBase
	Expr Expr
	Type TypeExpr
}

// NonNilAssertExpr represents a non-nil assertion: expr!
type NonNilAssertExpr struct {
	ExprBase
	Expr Expr
}

// TypeAnnotation holds optional type information for variables/parameters.
type TypeAnnotation struct {
	Type TypeExpr
}
