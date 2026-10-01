package b

import "a"

func ignoreUsage(c a.Code) {
	_ = c /* want "GOVD002: unused ignore directive" */ //govo:ignore // nothing to suppress

	_ = c == "A" /* want "GOVD002: unused ignore rule GOV001" */ //govo:ignore GOV001,GOV003 // only GOV003 applies

	_ = c == "B" /* want `GOVD001: unknown ignore rule "GOVO003"` "GOV003" */ //govo:ignore GOVO003 // previous rule ID

	_ = c == "C" /* want "GOVD001: ignore rule GOVD002 cannot be suppressed" "GOV003" */ //govo:ignore GOVD002 // not suppressible

	/* want `GOVD001: unknown govo directive "unknown"` */ //govo:unknown

	//govo:ignore GOVD001 // directive diagnostics can be suppressed
	_ = []int{
		//govo:unknown
		1,
	}
}

func redundantIgnores(c a.Code, s string) {
	//govo:ignore GOV003 // the first one is kept
	//govo:ignore GOV003 // want "GOVD004: redundant ignore rule GOV003; another ignore directive already suppresses it here"
	_ = c == "A"

	//govo:ignore GOV003,GOV003 // want "GOVD004: redundant ignore rule GOV003; it is listed more than once"
	_ = c == "B"

	//govo:ignore // the first one is kept
	//govo:ignore // want "GOVD004: redundant ignore directive; another ignore directive already suppresses all rules here"
	_ = c == "C"

	// Listed rule IDs take precedence over an ignore of all rules, wherever
	// it appears.
	//
	//govo:ignore // want "GOVD004: redundant ignore directive; another ignore directive lists the rules to suppress here"
	//govo:ignore GOV003 // the listed rule is kept
	_ = c == "D"

	//govo:ignore GOV001 // the listed rule is kept
	//govo:ignore // want "GOVD004: redundant ignore directive; another ignore directive lists the rules to suppress here"
	_ = a.Code(s)

	// Ignores of different targets are independent, and an ignore of
	// different rules for the same target is not redundant.
	//
	//govo:ignore GOV001 // the conversion
	//govo:ignore GOV003 // the comparison
	_ = a.Code(s) == "E"

	//govo:ignore GOV003 // another target
	_ = c == "F"

	_ = c == "G" //govo:ignore GOV003 // a trailing ignore of another target

	// A redundant rule that suppresses nothing is reported only as
	// redundant.
	//
	//govo:ignore GOV001 // want "GOVD002: unused ignore rule GOV001"
	//govo:ignore GOV001 // want "GOVD004: redundant ignore rule GOV001; another ignore directive already suppresses it here"
	_ = c
}
