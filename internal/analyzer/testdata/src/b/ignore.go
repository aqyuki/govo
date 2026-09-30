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
