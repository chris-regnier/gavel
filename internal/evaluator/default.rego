package gavel.gate

import rego.v1

default decision := "review"

decision := "reject" if {
    some result in input.runs[0].results
    result.level == "error"
    result.properties["gavel/confidence"] > 0.8
}

decision := "merge" if {
    count(input.runs[0].results) == 0
}
