// Thin client for the temis HTTP API (service/http.go). Requests are same-origin
// in dev thanks to the Vite proxy (see vite.config.js), so these paths are bare.
// An optional bearer token is forwarded on the gated /v1 calls.

function authHeaders(token, extra) {
  const h = { ...(extra || {}) };
  if (token) h['Authorization'] = 'Bearer ' + token;
  return h;
}

// Reads a problem+json body if present, else a generic HTTP message.
function errorText(resp, body) {
  if (body && (body.detail || body.title)) {
    return (body.code ? body.code + ': ' : '') + (body.detail || body.title);
  }
  return 'HTTP ' + resp.status;
}

async function asJson(resp) {
  const body = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new Error(errorText(resp, body));
  return body;
}

// POST /v1/models — compile + cache the model, returns { modelId, decisions,
// inputs, diagnostics }.
export async function loadModel(xml, token) {
  const resp = await fetch('/v1/models', {
    method: 'POST',
    headers: authHeaders(token, { 'Content-Type': 'application/xml' }),
    body: xml,
  });
  return asJson(resp);
}

// POST /v1/models/{id}/evaluate — evaluate one decision against the given input.
export async function evaluate(modelId, decision, input, token) {
  const resp = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/evaluate', {
    method: 'POST',
    headers: authHeaders(token, { 'Content-Type': 'application/json' }),
    body: JSON.stringify({ decision, input }),
  });
  return asJson(resp);
}
