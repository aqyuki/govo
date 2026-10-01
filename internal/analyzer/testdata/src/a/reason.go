package a

// Text after // in a directive is not parsed as arguments.

//govo:protect // a code of an external system
type Ticket string // want Ticket:"&{}"

//govo:protect// written without a space
type Weight uint // want Weight:"&{}"

//govo:factory Ticket // needed for compatibility
func NewTicket(s string) Ticket { return Ticket(s) }

//govo:converter Ticket // read by the exporter
func (t Ticket) String() string { return string(t) }

//govo:op Ticket // trims the code
func (t Ticket) Short() Ticket { return Ticket(string(t)[:1]) }

//govo:factory Weight// written without a space
//govo:scalar Weight // doubles the weight
func (w Weight) Double() Weight { return w * 2 }

// The type name may still be omitted before a reason only when the file
// declares exactly one protected type.
//
/* want "GOVD001: factory requires a type name unless this file declares exactly one protected type" */ //govo:factory // no type name
func ticketOf(s string) Ticket {
	return Ticket(s) // want "GOV001: direct construction of protected type Ticket"
}

// A reason does not hide an unknown type name.
//
/* want "GOVD001: converter: Unknown is not a protected type declared in this file" */ //govo:converter Ticket Unknown // reason
func describeTicket(t Ticket) string { return string(t) }

// Arguments of protect are still reported before a reason.
//
/* want "GOVD001: protect does not accept arguments" */ //govo:protect extra // reason
type notProtected string
