// attachFeelField is the one entry point every FEEL input/textarea in the
// modeler should go through. It lays the syntax-highlighting backdrop under the
// field (attachHighlighter) and wires the caret-anchored completion dropdown
// (attachCompletion) in a single call, so the two can never drift apart or be
// forgotten when a new boxed-expression editor is added — the whole point of a
// shared primitive rather than repeating the pair in every dialog (ADR-0016).
//
// names is read lazily on every render/open, so callers pass a live provider of
// the in-scope variables (e.g. an iterator body that also sees its loop
// variable). Read-only fields get highlighting but no completion — there is
// nothing to type into them. extra adds context-specific completions that are
// not model variables (e.g. the `?` input value of a decision-table cell).

import { attachCompletion, feelItems, type CompletionItem } from './complete'
import { attachHighlighter } from './highlight'
import { attachSignatureHint } from './signature'

export function attachFeelField(
  field: HTMLInputElement | HTMLTextAreaElement,
  names: () => string[],
  opts?: { readOnly?: boolean; extra?: () => CompletionItem[] },
): { refresh: () => void } {
  const hl = attachHighlighter(field, names)
  if (!opts?.readOnly) {
    attachCompletion(field, () => feelItems(names(), opts?.extra?.() ?? []))
    // Beginner aid: a translucent signature/construct hint above the field.
    attachSignatureHint(field)
  }
  return hl
}
