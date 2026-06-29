// Thin client for the temis /v1 endpoints the modeler needs. temis stays the
// model authority (ADR-0016): the browser fetches the decision requirements
// graph rather than parsing DMN XML itself.

export type GraphNode = { id: string; type: string; name: string }
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
