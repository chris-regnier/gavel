class RelatedLocation {
  filePath string @description("Path to the file containing the related code")
  startLine int @description("Line number where the related code starts")
  endLine int? @description("Optional line number where the related code ends")
  message string? @description("Brief explanation of why this location is related (e.g. 'Unsanitized input originates here')")
}

class FlowStep {
  filePath string @description("Path to the file containing this step in the flow")
  startLine int @description("Line number for this step")
  endLine int? @description("Optional end line for this step")
  message string @description("What happens at this step (e.g. 'User input read from request body', 'Concatenated into SQL query string')")
}

class CodeFlow {
  message string? @description("Optional one-sentence summary of the entire flow (e.g. 'Tainted user input reaches SQL query without sanitization')")
  steps FlowStep[] @description("Ordered sequence of locations forming the data or control flow path. Each step describes one hop from source to sink. At least two steps required for the flow to be useful.")
}

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
  fixReplacementText string? @description("Optional raw replacement text for lines startLine through endLine of this finding. Match the original indentation. Leave empty for vague or structural findings. Raw text only — no markdown code fences, no prose.")
  relatedLocations RelatedLocation[]? @description("Other code locations that meaningfully relate to this finding (e.g. data origin for a taint sink, the definition of a vulnerable callee, a related call site). Omit for self-contained findings. Quality over quantity — only include locations that genuinely help triage.")
  codeFlows CodeFlow[]? @description("Ordered data- or control-flow paths showing how the issue arises step-by-step (e.g. user input → propagation → sink). Populate only for findings where the multi-step path is essential to understand the issue (typically taint-style security findings) and you can identify each hop with confidence. Omit for self-contained findings, single-line bugs, or when steps would be speculative.")
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
    Analyze the content against these specific policies. Only report genuine violations.
    If a policy doesn't apply to this content, don't force a finding.

    {{ policies }}

    ===== CONTENT TO ANALYZE =====
    {{ code }}

    ===== INSTRUCTIONS =====
    For each issue you find:
    1. Identify the exact line numbers where it occurs
    2. Write a concise message (one sentence)
    3. Provide a detailed explanation following your persona's tone
    4. Suggest a specific, actionable recommendation
    5. Assign an appropriate confidence level based on the guidance above
    6. If you can express a machine-applicable fix, populate fixReplacementText with
       the raw replacement text that should replace lines startLine through endLine
       (inclusive). Preserve the original indentation of the flagged region so the
       replacement can be applied as-is. Provide raw text only — no markdown code
       fences, no prose. If the fix is vague or structural (e.g. "consider
       restructuring this module"), leave fixReplacementText empty.
    7. If the finding meaningfully relates to other code locations (e.g. the
       origin of unsanitized input, the definition of a vulnerable callee, or
       a related call site), populate relatedLocations with each location and a
       brief message explaining the relationship. Omit relatedLocations for
       findings that are self-contained at a single location.
    8. If the issue is fundamentally a multi-step data or control flow (most
       commonly: tainted input flowing to a dangerous sink), populate codeFlows
       with the ordered hops the value takes. Each step needs a real file path
       and line number plus a short message describing what happens there
       ("user input read", "passed without sanitization", "concatenated into
       SQL"). Only emit a flow when you can identify each hop with confidence
       — leave codeFlows empty for self-contained findings, speculative paths,
       or anything you cannot trace. A flow with fewer than two steps is not
       useful; omit it.

    Only report genuine issues. Quality over quantity.

    {{ ctx.output_format }}
  "#
}
