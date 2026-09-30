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

var Postmaster = Email("postmaster@example.com") // want "GOVO001: direct construction of protected type Email"

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
	return Email(string(e)) // want "GOVO002: direct extraction from protected type Email"
}

//govo:converter Email
func (e Email) Domain() Domain {
	_ = Email(string(e))     // want "GOVO001: direct construction of protected type Email"
	return Domain(string(e)) // want "GOVO001: direct construction of protected type Domain"
}

// A marker grants permission only for the types it names.
//
//govo:factory Domain
func NewDomain(s string) Domain {
	_ = Email(s) // want "GOVO001: direct construction of protected type Email"
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
/* want "GOVO004: factory: Email is not returned by this API" */ //govo:factory Email
func broken(s string) string {
	_ = Email(s) // want "GOVO001: direct construction of protected type Email"
	return s
}

func unmarked(s string, e Email) {
	const local Email = "local"
	_ = Email(s)                         // want `GOVO001: direct construction of protected type Email; use a //govo:factory function$`
	_ = string(e)                        // want `GOVO002: direct extraction from protected type Email; use a //govo:converter function$`
	_ = func() Email { return Email(s) } // want "GOVO001: direct construction of protected type Email"
	_ = local
}

/* want "GOVO004: factory requires a function or method" */ //govo:factory Email
var _ = 0
