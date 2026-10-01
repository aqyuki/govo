package a

//govo:protect
type List[T any] []T // want List:"&{}"

// A phantom type parameter distinguishes IDs of different entities.
//
//govo:protect
type RefID[E any] string // want RefID:"&{}"

//govo:protect
type Money[C any] int64 // want Money:"&{}"

type (
	User  struct{}
	Order struct{}
	Yen   struct{}
)

// Instances may name the protected types of typed constants.
const RootID RefID[User] = "root"

// A generic factory constructs any instance.
//
//govo:factory List
func NewList[T any](xs []T) List[T] { return List[T](xs) }

// A factory that returns one instance is a factory of the type.
//
//govo:factory List
func NewInts(xs []int) List[int] { return List[int](xs) }

// So is a factory that returns an instance through an alias.
//
//govo:factory List
func NewStrings(xs []string) Strings { return Strings(xs) }

type Strings = List[string]

//govo:converter List
func (l List[T]) Raw() []T { return []T(l) }

//govo:converter List
func (l *List[T]) RawPtr() []T { return []T(*l) }

//govo:op List
func (l List[T]) Clone() List[T] {
	return List[T](append([]T(nil), []T(l)...))
}

//govo:factory RefID
func ParseID[E any](s string) (RefID[E], error) { return RefID[E](s), nil }

//govo:converter RefID
func (id RefID[E]) String() string { return string(id) }

// Converting between instances crosses the boundary like any other
// conversion between protected types.
//
//govo:factory RefID
//govo:converter RefID
func Rebind[F, E any](id RefID[E]) RefID[F] { return RefID[F](id) }

// A factory of an instance is still trusted only with direct conversions.
//
//govo:factory RefID
func Retag(id RefID[Order]) RefID[User] {
	return RefID[User](id) // want `GOV002: direct extraction from protected type RefID\[Order\]`
}

//govo:op Money
//govo:scalar Money
func (m Money[C]) Doubled() Money[C] { return m * 2 }

//govo:factory Money
func (m Money[C]) Next() Money[C] {
	return m + 1 // want `GOV003: untyped constant used as protected type Money\[C\]`
}

/* want "GOVD001: factory: List is not returned by this API" */ //govo:factory List
func Sum(xs []int) int { return len(List[int](xs)) } // want `GOV001: direct construction of protected type List\[int\]`

/* want "GOVD001: converter: RefID is not accepted by this API" */ //govo:converter RefID
func IDString(s string) string { return s }

func genericUnmarked[T any](xs []T, l List[T], id RefID[User]) {
	_ = List[T](xs) // want `GOV001: direct construction of protected type List\[T\]; use a //govo:factory function$`
	_ = []T(l)      // want `GOV002: direct extraction from protected type List\[T\]; use a //govo:converter function$`
	_ = List[T](l)
	_ = string(id) // want `GOV002: direct extraction from protected type RefID\[User\]`
}
