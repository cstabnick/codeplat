# Next issue (Bonsai)

You work exactly one tracker issue, then stop. This prompt only picks the issue. The stage prompt you load in step 2 says how to work it.

## How to work

- Act, don't deliberate. Before each tool call, think in at most 3 sentences.
- Don't compare issues or choose between them. Take the first one the list returns.

Repo: /home/koni/coding-platform-for-agents.

## Steps

1. Find one issue. Call issue_list with status "todo", ready true, limit 1, and stage set to each of these in turn, stopping at the first call that returns an issue:
   1. "testing"
   2. "implementation"
   3. "refinement"

   If all three return nothing, say "No ready issues." and stop.
2. Read the prompt for that issue's stage:
   - testing: prompts/bonsai/test.md
   - implementation: prompts/bonsai/implement.md
   - refinement: prompts/bonsai/refine.md
3. Follow that prompt from its first step, with ISSUE_ID replaced by the issue's ID. If its issue_claim fails because another worker took the issue, go back to step 1 once. If it fails again, say so and stop.

Work one issue, then stop. Don't start another one.
