import type { Anchor } from './api'
import { FEEL_TYPES } from './feeltypes'
import { openTableOverlay } from './table'
import { openBoxedContextOverlay } from './boxedcontext'
import { openListOverlay } from './list'
import { openRelationOverlay } from './relation'
import { openInvocationOverlay } from './invocation'
import { openIteratorOverlay } from './iterator'
import { openConditionalOverlay } from './conditional'
import { openFilterOverlay } from './filter'

// BoxedTarget locates a boxed expression to edit: the model, the anchor (a
// decision's logic or a BKM's encapsulated body) and an optional nested path
// (`at`, e.g. "entry.1"). names are the in-scope variables the expressions may
// reference; onSaved gets the saved model's id; readOnly opens for viewing only.
export type BoxedTarget = {
  modelId: string
  anchor: Anchor
  at?: string
  names: string[]
  onSaved?: (newModelId: string) => void
  typeOptions?: string[]
  readOnly?: boolean
}

// openBoxed opens the boxed editor matching kind on the target (WP-66). It is the
// one place that maps a boxed kind to its editor, shared by the BKM-body dispatch
// and the context drill-in. It returns false for a kind with no editor (e.g. a
// nested function), so the caller can fall back to a read-only note.
export function openBoxed(kind: string, t: BoxedTarget): boolean {
  const typeOptions = t.typeOptions ?? FEEL_TYPES
  const { modelId, anchor, at, names, onSaved, readOnly } = t
  const o = { anchor, at, readOnly }
  switch (kind) {
    case 'table':
      void openTableOverlay(modelId, anchor.id, onSaved, typeOptions, { anchor, at, readOnly, scope: names })
      return true
    case 'context':
      void openBoxedContextOverlay(modelId, anchor.id, names, onSaved, { typeOptions, anchor, at, readOnly })
      return true
    case 'list':
      void openListOverlay(modelId, anchor.id, names, onSaved, o)
      return true
    case 'relation':
      void openRelationOverlay(modelId, anchor.id, names, onSaved, o)
      return true
    case 'invocation':
      void openInvocationOverlay(modelId, anchor.id, names, onSaved, o)
      return true
    case 'iterator':
      void openIteratorOverlay(modelId, anchor.id, names, onSaved, o)
      return true
    case 'conditional':
      void openConditionalOverlay(modelId, anchor.id, names, onSaved, o)
      return true
    case 'filter':
      void openFilterOverlay(modelId, anchor.id, names, onSaved, o)
      return true
    default:
      return false
  }
}

// joinAt appends a locator step to a parent path ("" root → "entry.1";
// "entry.1" → "entry.1/item.0").
export function joinAt(parent: string | undefined, step: string): string {
  return parent ? parent + '/' + step : step
}
