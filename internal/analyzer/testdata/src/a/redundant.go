package a

//govo:protect
type Score int // want Score:"&{}"

// A directive that repeats a type for the same function is redundant, and
// the later one is reported.
//
//govo:factory Score
/* want "GOVD004: factory: Score is named more than once for this function" */ //govo:factory Score
func NewScore(n int) Score { return Score(n) }

/* want "GOVD004: converter: Score is named more than once for this function" */ //govo:converter Score Score
func (s Score) Int() int { return int(s) }

// A directive without a type name names the only protected type in the file.
//
//govo:op
/* want "GOVD004: op: Score is named more than once for this function" */ //govo:op Score
func (s Score) Plus(n int) Score { return Score(int(s) + n) }

// An op permits both construction and extraction, so a factory or
// converter of the same type is redundant wherever it appears.
//
/* want "GOVD004: factory: Score is already permitted by op Score" */ //govo:factory Score
//govo:op Score
/* want "GOVD004: converter: Score is already permitted by op Score" */ //govo:converter
func (s Score) Minus(n int) Score { return Score(int(s) - n) }

//govo:op Score
/* want "GOVD004: factory: Score is already permitted by op Score" */ //govo:factory Score Score
func (s Score) Negated() Score { return Score(-int(s)) }

//govo:op Score
//govo:scalar Score
/* want "GOVD004: scalar: Score is named more than once for this function" */ //govo:scalar Score
func (s Score) Doubled() Score { return s * 2 }

// A factory and a converter grant different permissions, so together they
// are not redundant.
//
//govo:factory Score
//govo:converter Score
func (s Score) Abs() Score {
	if s < Score(0) {
		return Score(-int(s))
	}

	return s
}

// Only a valid marker makes another redundant.
//
/* want "GOVD001: op: Score is not accepted by this API" */ //govo:op Score
//govo:factory Score
func ParseScore(n int) Score { return Score(n) }

// Directives of different functions are independent.
//
//govo:factory Score
func ZeroScore() Score { return Score(0) }

// A redundant directive still grants its permission, and an ignore covers it.
//
//govo:ignore GOVD004 // kept while callers migrate
//govo:factory Score
//govo:factory Score
func OneScore() Score { return Score(1) }
