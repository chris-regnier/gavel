package gavel.gate

import rego.v1

default decision := "review"

_suppressed(result) if {
	suppressions := object.get(result, "suppressions", [])
	count(suppressions) > 0
}

# _is_pre_existing returns true for results that baseline comparison has
# identified as already present in the baseline ("unchanged"). Gating
# should ignore these so that a PR is not penalized for noise that
# predates it.
_is_pre_existing(result) if {
	object.get(result, "baselineState", "") == "unchanged"
}

# _is_fixed returns true for results that baseline comparison has
# identified as present in the baseline but absent from the current run
# ("absent"). These represent findings that were fixed; gating should
# ignore them so that a fix is not conflated with a regression.
_is_fixed(result) if {
	object.get(result, "baselineState", "") == "absent"
}

# unsuppressed_results is all results in the current run that do not
# carry an explicit suppression. It is kept as a primitive that custom
# policies can consume if they want to reason about the full
# (post-suppression, pre-baseline-filter) set.
unsuppressed_results contains result if {
	some result in input.runs[0].results
	not _suppressed(result)
}

# actionable_results is the set that gating decisions should consider:
# unsuppressed findings that are not pre-existing noise or already-fixed
# findings. When analyze is run without --baseline no result carries a
# baselineState, so _is_pre_existing/_is_fixed never match and this set
# equals unsuppressed_results — preserving existing semantics for
# callers that have not adopted baseline comparison.
actionable_results contains result if {
	some result in unsuppressed_results
	not _is_pre_existing(result)
	not _is_fixed(result)
}

decision := "reject" if {
	some result in actionable_results
	result.level == "error"
	result.properties["gavel/confidence"] > 0.85
}

decision := "merge" if {
	count(actionable_results) == 0
}
