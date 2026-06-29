// Thin client for the temis /v1 endpoints the modeler needs. temis stays the
// model authority (ADR-0016): the browser fetches the decision requirements
// graph rather than parsing DMN XML itself.

// dataType is the node's resolved FEEL type; varName a decision's output-variable
// name. x/y/width/height are present only when the model carries DMNDI (absent →
// the client auto-lays-out the graph).
export type GraphNode = {
  id: string
  type: string
  name: string
  dataType?: string
  varName?: string
  x?: number
  y?: number
  width?: number
  height?: number
}
export type GraphEdge = { type: string; source: string; target: string }
export type Graph = { nodes: GraphNode[]; edges: GraphEdge[] }

export type ModelSummary = { modelId: string; name?: string; decisions: string[]; inputs: string[] }

export async function listModels(): Promise<ModelSummary[]> {
  const r = await fetch('/v1/models')
  if (!r.ok) throw new Error('Modelle laden fehlgeschlagen (HTTP ' + r.status + ')')
  const body = (await r.json()) as { models?: ModelSummary[] }
  return body.models ?? []
}

export async function getGraph(modelId: string): Promise<Graph> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/graph')
  if (!r.ok) throw new Error('Graph laden fehlgeschlagen (HTTP ' + r.status + ')')
  return (await r.json()) as Graph
}

// NodeEdit is one node's persistable edit; omitted fields stay unchanged. x/y are
// only honoured for models that carry DMNDI (otherwise positions auto-lay-out).
export type NodeEdit = { id: string; name?: string; dataType?: string; x?: number; y?: number }

// saveModel patches the model's DMN XML with the given node edits (positions,
// names, types), recompiles it server-side and returns the SAVED model's id —
// the content hash changes, so the caller switches to the new revision. Decision
// logic and untouched diagram interchange are preserved (ADR-0016, Edit→Save).
export async function saveModel(modelId: string, nodes: NodeEdit[]): Promise<string> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/save', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ nodes }),
  })
  if (!r.ok) throw new Error('Speichern fehlgeschlagen (HTTP ' + r.status + ')')
  const body = (await r.json()) as { modelId: string }
  return body.modelId
}
