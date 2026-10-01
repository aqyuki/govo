package a

//govo:protect
type Email string // want Email:"&{}"

//govo:protect
type Domain string // want Domain:"&{}"

// Protected constants may be declared in the type declaration file,
// including with explicit conversions.
const (
	Admin        Email = "admin@example.com"
	Support            = Email("support@example.com")
	DefaultEmail       = Admin
)

var Postmaster = Email("postmaster@example.com") // want "GOV001: direct construction of protected type Email"

//govo:factory Email
func NewEmail(s string) (Email, error) { return Email(s), nil }

//govo:factory Email
func mustEmail(s string) Email { return Email(s) }

//govo:converter Email
func (e Email) String() string { return string(e) }

// A method that both extracts and constructs needs both markers.
//
//govo:factory Email
//govo:converter Email
func (e Email) Lower() Email {
	raw := string(e)
	return Email(raw)
}

//govo:factory Email
func (e Email) Upper() Email {
	return Email(string(e)) // want "GOV002: direct extraction from protected type Email"
}

//govo:converter Email
func (e Email) Domain() Domain {
	_ = Email(string(e))     // want "GOV001: direct construction of protected type Email"
	return Domain(string(e)) // want "GOV001: direct construction of protected type Domain"
}

// A marker grants permission only for the types it names.
//
//govo:factory Domain
func NewDomain(s string) Domain {
	_ = Email(s) // want "GOV001: direct construction of protected type Email"
	return Domain(s)
}

// A conversion between protected types needs permission for both.
//
//govo:factory Domain
//govo:converter Email
func DomainOf(e Email) Domain { return Domain(e) }

// Function literals share the permission of the enclosing declaration.
//
//govo:factory Email
func builtEmail(s string) Email {
	build := func() Email { return Email(s) }
	return build()
}

// A type that fails its shape check grants nothing.
//
/* want "GOVD001: factory: Email is not returned by this API" */ //govo:factory Email
func broken(s string) string {
	_ = Email(s) // want "GOV001: direct construction of protected type Email"
	return s
}

func unmarked(s string, e Email) {
	const local Email = "local"
	_ = Email(s)                         // want `GOV001: direct construction of protected type Email; use a //govo:factory function$`
	_ = string(e)                        // want `GOV002: direct extraction from protected type Email; use a //govo:converter function$`
	_ = func() Email { return Email(s) } // want "GOV001: direct construction of protected type Email"
	_ = local
}

/* want "GOVD001: factory requires a function or method" */ //govo:factory Email
var _ = 0

// An ignore before other directives in a doc comment covers their
// diagnostics.
//
//govo:ignore GOVD001 // the marker names a type from another file
//govo:converter Missing
func ignoredMarker(e Email) string { return "" }

//govo:ignore GOVD001 // kept for an older signature
//govo:factory Email
func ignoredShape(s string) string {
	_ = Email(s) // want "GOV001: direct construction of protected type Email"
	return s
}

//govo:ignore GOVD001 // the directive is checked separately
//govo:protect Extra
type ignoredProtect string

// An ignore after a directive does not cover it.
//
/* want "GOVD001: converter: Missing is not a protected type declared in this file" */ //govo:converter Missing
/* want "GOVD002: unused ignore rule GOVD001" */ //govo:ignore GOVD001 // too late
func lateIgnore(e Email) string { return "" }
