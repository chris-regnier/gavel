class Finding {
  ruleId string @description("The policy name this finding relates to")
  level string @description("One of: error, warning, note, none")
  message string @description("Concise description of the issue")
  filePath string @description("Path to the file containing the issue")
  startLine int @description("Line number where the issue starts")
  endLine int @description("Line number where the issue ends")
  recommendation string @description("Suggested fix or action")
  explanation string @description("Longer reasoning about why this is an issue")
  confidence float @description("0.0 to 1.0, how confident you are in this finding")
}

function AnalyzeCode(code: string, policies: string, additionalContext: string) -> Finding[] {
  client OpenRouter
  prompt #"
    You are a precise code analyzer. Analyze the following code against the given policies.
    For each policy violation found, produce a finding. If no violations are found
    for a policy, do not produce a finding for it.

    Be precise about line numbers. Be concise in messages. Be thorough in explanations.
    Set confidence based on how certain you are — use lower confidence for ambiguous cases.

    Only report genuine issues. Do not fabricate findings.

    Policies:
    ---
    {{ policies }}
    ---

    {% if additionalContext %}
    {{ additionalContext }}
    ---
    {% endif %}

    Code:
    ---
    {{ code }}
    ---

    {{ ctx.output_format }}
  "#
}
