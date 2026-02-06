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

function AnalyzeCode(
  code: string,
  policies: string,
  personaPrompt: string,
  additionalContext: string
) -> Finding[] {
  client OpenRouter
  prompt #"
    {{ personaPrompt }}

    {% if additionalContext != "" %}
    ===== ADDITIONAL CONTEXT =====
    The following context may be relevant to your analysis:

    {{ additionalContext }}

    ===== END CONTEXT =====
    {% endif %}

    ===== POLICIES TO CHECK =====
    Analyze the code against these specific policies. Only report genuine violations.
    If a policy doesn't apply to this code, don't force a finding.

    {{ policies }}

    ===== CODE TO ANALYZE =====
    {{ code }}

    ===== INSTRUCTIONS =====
    For each policy violation or issue you find:
    1. Identify the exact line numbers where it occurs
    2. Write a concise message (one sentence)
    3. Provide a detailed explanation following your persona's tone
    4. Suggest a specific, actionable recommendation
    5. Assign an appropriate confidence level based on the guidance above

    Only report genuine issues. Quality over quantity.

    {{ ctx.output_format }}
  "#
}
