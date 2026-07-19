---
name: grill-with-docs
description: A relentless interview to sharpen a plan or design, which also creates docs (ADR's and glossary) as we go.
disable-model-invocation: true
---

# Grill With Docs

Run a `/grilling` session while actively using the `/domain-modeling`
discipline. This skill is for turning a fuzzy plan, feature idea, technical
design, or product decision into shared understanding and durable project
context.

Do not treat this as a passive Q&A. The purpose is to make the plan sharper,
catch contradictions, name the domain concepts correctly, and write down the
decisions that future agents and humans must inherit.

## When to use

Use this skill when:

- A plan or design sounds plausible but still has vague terms, hidden
  assumptions, unclear scope, or weak success criteria.
- The team needs to align on domain language before writing a spec or tickets.
- A feature touches concepts that already exist in the codebase or docs and
  may conflict with existing terminology.
- A decision may need an ADR because it is hard to reverse, surprising without
  context, or the result of a real trade-off.

## Process

1. Read the relevant existing docs first.
   - Look for `CONTEXT.md`, `CONTEXT-MAP.md`, ADRs, specs, tickets, and nearby
     implementation notes.
   - If a fact can be discovered from the repo or tools, look it up instead of
     asking the user.

2. Start a grilling loop.
   - Ask one question at a time.
   - For each question, provide your recommended answer.
   - Walk the decision tree in dependency order. Resolve upstream choices before
     asking about downstream implementation details.
   - Do not act on the plan until the user confirms shared understanding.

3. Maintain the domain model as the conversation progresses.
   - Challenge fuzzy or overloaded terms immediately.
   - If the user says "account", "workspace", "project", "task", "agent", or
     another domain term, confirm whether it matches the existing glossary.
   - When a term is resolved, update the appropriate domain glossary document.
   - Keep glossary entries free of implementation details.

4. Record architectural decisions only when they deserve it.
   - Create an ADR only if the decision is hard to reverse, surprising without
     context, and based on a real trade-off.
   - If it is just a small implementation note, put it in the spec or handoff
     instead of creating unnecessary decision documents.

5. End with a decision-ready handoff.
   - Summarize what is now clear.
   - List remaining open questions.
   - Name the docs that were created or updated.
   - State whether the work is ready for spec, needs more grilling, or should
     stop.

## Output fields

Return structured output with these fields:

- `clarification_doc_id`: docID for the clarified plan or design.
- `domain_context_doc_id`: docID for glossary/domain updates, if any.
- `adr_doc_ids`: list of ADR docIDs created or updated, if any.
- `open_questions`: unresolved questions that still block execution.
- `readiness`: `ready_for_spec`, `needs_more_grilling`, or `stop`.

## Guardrails

- Do not ask multiple questions at once.
- Do not create docs just to look productive. Create or update docs only when
  the team learned something worth preserving.
- Do not let the user skip over ambiguous domain language. Ambiguous language
  becomes expensive later.
- Do not convert the discussion into implementation until the plan is stable.
