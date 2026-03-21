package gavel.gate

import rego.v1

default decision := "review"

_suppressed(result) if {
	suppressions := object.get(result, "suppressions", [])
	count(suppressions) > 0
}

unsuppressed_results contains result if {
	some result in input.runs[0].results
	not _suppressed(result)
}

decision := "reject" if {
	some result in unsuppressed_results
	result.level == "error"
	result.properties["gavel/confidence"] > 0.8
}

decision := "merge" if {
	count(unsuppressed_results) == 0
}
