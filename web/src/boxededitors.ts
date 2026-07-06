import { createDecisionTable, createBoxedContext, createBoxedConditional, createBoxedList, createBoxedRelation, createBoxedFilter, createBoxedIterator, createBoxedInvocation, type Anchor, type ModelDetail } from './api'
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

// BoxedType is one row of the boxed-type registry (WP-142): the single source of
// truth for a decision's boxed logic kind. Adding a boxed kind is one entry here
// (plus its overlay in openBoxed and its icon in dmn-context-pad) rather than the
// ~13 hardcoded sites it used to touch across main.ts/canvas.ts/dmn-context-pad.ts.
// hasFlag is the GraphNode/Shape property that marks a decision as carrying this
// kind. editTitle/createTitle are the context-pad labels; statusCreating/Created
// are the app-shell status messages while a fresh one is created. create is the
// api endpoint that gives an undecided decision a fresh instance — null for
// literal (materialized on save, not up front).
export type BoxedType = {
  kind: string
  hasFlag: 'hasTable' | 'hasLiteral' | 'hasContext' | 'hasConditional' | 'hasList' | 'hasRelation' | 'hasFilter' | 'hasIterator' | 'hasInvocation'
  editTitle: string
  createTitle: string
  statusCreating: string
  statusCreated: string
  create: ((modelId: string, decision: string) => Promise<ModelDetail>) | null
}

// BOXED_TYPES is the ordered registry of a decision's boxed logic kinds. A decision
// carries at most one, so lookups match the first entry whose hasFlag is set.
export const BOXED_TYPES: BoxedType[] = [
  { kind: 'table', hasFlag: 'hasTable', editTitle: 'Decision Table anzeigen', createTitle: 'Decision Table anlegen', statusCreating: 'legt Tabelle an …', statusCreated: 'Tabelle angelegt ✓', create: createDecisionTable },
  { kind: 'literal', hasFlag: 'hasLiteral', editTitle: 'FEEL-Ausdruck anzeigen', createTitle: 'FEEL-Ausdruck anlegen', statusCreating: '', statusCreated: '', create: null },
  { kind: 'context', hasFlag: 'hasContext', editTitle: 'Boxed Context bearbeiten', createTitle: 'Boxed Context anlegen', statusCreating: 'legt Boxed Context an …', statusCreated: 'Boxed Context angelegt ✓', create: createBoxedContext },
  { kind: 'conditional', hasFlag: 'hasConditional', editTitle: 'Conditional (if/then/else) bearbeiten', createTitle: 'Conditional (if/then/else) anlegen', statusCreating: 'legt Conditional an …', statusCreated: 'Conditional angelegt ✓', create: createBoxedConditional },
  { kind: 'list', hasFlag: 'hasList', editTitle: 'Liste bearbeiten', createTitle: 'Liste anlegen', statusCreating: 'legt Liste an …', statusCreated: 'Liste angelegt ✓', create: createBoxedList },
  { kind: 'relation', hasFlag: 'hasRelation', editTitle: 'Relation bearbeiten', createTitle: 'Relation anlegen', statusCreating: 'legt Relation an …', statusCreated: 'Relation angelegt ✓', create: createBoxedRelation },
  { kind: 'filter', hasFlag: 'hasFilter', editTitle: 'Filter bearbeiten', createTitle: 'Filter anlegen', statusCreating: 'legt Filter an …', statusCreated: 'Filter angelegt ✓', create: createBoxedFilter },
  { kind: 'iterator', hasFlag: 'hasIterator', editTitle: 'Iteration (for/some/every) bearbeiten', createTitle: 'Iteration (for/some/every) anlegen', statusCreating: 'legt Iteration an …', statusCreated: 'Iteration angelegt ✓', create: createBoxedIterator },
  { kind: 'invocation', hasFlag: 'hasInvocation', editTitle: 'Invocation (Funktions-/BKM-Aufruf) bearbeiten', createTitle: 'Invocation (Funktions-/BKM-Aufruf) anlegen', statusCreating: 'legt Invocation an …', statusCreated: 'Invocation angelegt ✓', create: createBoxedInvocation },
]

// boxedTypeFor returns the registry entry for the boxed kind a node carries (the
// first whose hasFlag is set on the node), or undefined for an undecided node.
export function boxedTypeFor(node: Record<string, unknown>): BoxedType | undefined {
  return BOXED_TYPES.find((b) => node[b.hasFlag])
}
