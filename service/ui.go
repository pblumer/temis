package service

import "net/http"

// playgroundPage is the self-contained DMN playground: a single HTML document
// with inline CSS and vanilla JavaScript and no external assets, so it renders
// offline and adds no Go or vendored-asset dependencies (unlike the CDN-backed
// Swagger UI at /docs). It drives the existing /v1 endpoints from the browser:
// load a model (POST /v1/models) to discover its decisions and inputs, then
// evaluate a chosen decision (POST /v1/models/{id}/evaluate). An optional bearer
// token is sent as Authorization on the gated calls.
//
// The script avoids JS template literals on purpose: the page lives in a Go raw
// string literal, so it must contain no backticks.
const playgroundPage = `<!DOCTYPE html>
<html lang="de">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Temis — DMN Playground</title>
  <link rel="icon" href="data:,">
  <style>
    :root {
      --bg: #0f1115; --panel: #1a1d24; --border: #2b303b; --fg: #e6e9ef;
      --muted: #98a2b3; --accent: #5b8def; --ok: #3fb950; --err: #f85149;
      --warn: #d29922; --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0; background: var(--bg); color: var(--fg);
      font: 15px/1.5 system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
    }
    header {
      padding: 16px 24px; border-bottom: 1px solid var(--border);
      display: flex; align-items: baseline; gap: 12px; flex-wrap: wrap;
    }
    header h1 { font-size: 18px; margin: 0; }
    header .sub { color: var(--muted); font-size: 13px; }
    header a { color: var(--accent); text-decoration: none; margin-left: auto; }
    header a:hover { text-decoration: underline; }
    main { max-width: 1100px; margin: 0 auto; padding: 24px;
      display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
    @media (max-width: 820px) { main { grid-template-columns: 1fr; } }
    .panel { background: var(--panel); border: 1px solid var(--border);
      border-radius: 10px; padding: 16px; }
    .panel h2 { font-size: 14px; margin: 0 0 12px; text-transform: uppercase;
      letter-spacing: .04em; color: var(--muted); }
    label { display: block; font-size: 13px; color: var(--muted); margin: 12px 0 4px; }
    textarea, input, select {
      width: 100%; background: var(--bg); color: var(--fg);
      border: 1px solid var(--border); border-radius: 6px; padding: 8px 10px;
      font-family: var(--mono); font-size: 13px;
    }
    textarea { resize: vertical; min-height: 180px; }
    textarea#xml { min-height: 240px; }
    button {
      background: var(--accent); color: #fff; border: none; border-radius: 6px;
      padding: 9px 16px; font-size: 14px; font-weight: 600; cursor: pointer;
    }
    button.secondary { background: transparent; color: var(--accent);
      border: 1px solid var(--border); }
    button:disabled { opacity: .5; cursor: not-allowed; }
    .row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-top: 12px; }
    .grow { flex: 1; }
    table { width: 100%; border-collapse: collapse; font-size: 13px; }
    th, td { text-align: left; padding: 6px 8px; border-bottom: 1px solid var(--border);
      vertical-align: top; }
    th { color: var(--muted); font-weight: 600; }
    td input { padding: 5px 8px; }
    .kv-key { font-family: var(--mono); color: var(--fg); white-space: nowrap; }
    pre { background: var(--bg); border: 1px solid var(--border); border-radius: 6px;
      padding: 12px; overflow: auto; font-size: 12.5px; margin: 0; }
    .muted { color: var(--muted); font-size: 13px; }
    .pill { display: inline-block; font-family: var(--mono); font-size: 12px;
      background: var(--bg); border: 1px solid var(--border); border-radius: 999px;
      padding: 2px 10px; margin: 2px 4px 2px 0; }
    .status { margin-top: 12px; font-size: 13px; min-height: 20px; }
    .status.ok { color: var(--ok); } .status.err { color: var(--err); }
    .diag { border-left: 3px solid var(--border); padding: 4px 10px; margin: 6px 0;
      font-size: 13px; }
    .diag.error { border-color: var(--err); } .diag.warning { border-color: var(--warn); }
    .diag.info { border-color: var(--accent); }
    .out-table td:first-child { font-family: var(--mono); color: var(--accent); }
    details summary { cursor: pointer; color: var(--muted); font-size: 13px; margin-top: 12px; }
  </style>
</head>
<body>
  <header>
    <h1>Temis — DMN Playground</h1>
    <span class="sub">Modell laden · Decision auswerten — direkt im Browser</span>
    <a href="/docs">API-Doku (Swagger UI) →</a>
  </header>
  <main>
    <section class="panel">
      <h2>1 · Modell</h2>
      <label for="xml">DMN-XML einfügen</label>
      <textarea id="xml" spellcheck="false" placeholder="<definitions ...> … </definitions>"></textarea>
      <div class="row">
        <input type="file" id="file" accept=".dmn,.xml,application/xml,text/xml" class="grow">
      </div>
      <label for="token">Bearer-Token (optional)</label>
      <input type="text" id="token" placeholder="nur nötig, wenn der Server -token verlangt">
      <div class="row">
        <button id="load">Modell laden</button>
        <span id="loadStatus" class="status"></span>
      </div>
      <div id="indexBox" style="display:none">
        <label>Erkannte Inputs</label>
        <div id="inputPills" class="muted"></div>
      </div>
    </section>

    <section class="panel">
      <h2>2 · Auswerten</h2>
      <label for="decision">Decision</label>
      <select id="decision"><option value="">— erst Modell laden —</option></select>
      <label>Eingabewerte
        <span class="muted">(Werte werden als JSON interpretiert; sonst als Text)</span>
      </label>
      <table id="inputTable"><tbody></tbody></table>
      <details>
        <summary>Rohes JSON bearbeiten</summary>
        <textarea id="inputJson" spellcheck="false" placeholder='{ "Season": "Winter" }'></textarea>
        <div class="muted">Wird beim Auswerten bevorzugt, wenn ausgefüllt.</div>
      </details>
      <div class="row">
        <button id="eval" disabled>Auswerten</button>
        <button id="evalStateless" class="secondary">Stateless auswerten</button>
        <span id="evalStatus" class="status"></span>
      </div>
      <div id="resultBox" style="display:none">
        <h2 style="margin-top:20px">Ergebnis</h2>
        <table class="out-table" id="outTable"><tbody></tbody></table>
        <div id="diags"></div>
        <details>
          <summary>Rohe Antwort</summary>
          <pre id="rawResult"></pre>
        </details>
      </div>
    </section>
  </main>

  <script>
    var modelId = null;
    var $ = function (id) { return document.getElementById(id); };

    function authHeaders(extra) {
      var h = extra || {};
      var t = $('token').value.trim();
      if (t) { h['Authorization'] = 'Bearer ' + t; }
      return h;
    }

    function setStatus(el, msg, kind) {
      el.textContent = msg;
      el.className = 'status' + (kind ? ' ' + kind : '');
    }

    // Read a problem+json detail if the body is one, else a generic message.
    function errorText(resp, body) {
      if (body && (body.detail || body.title)) {
        return (body.code ? body.code + ': ' : '') + (body.detail || body.title);
      }
      return 'HTTP ' + resp.status;
    }

    $('file').addEventListener('change', function (ev) {
      var f = ev.target.files[0];
      if (!f) { return; }
      var rd = new FileReader();
      rd.onload = function () { $('xml').value = rd.result; };
      rd.readAsText(f);
    });

    $('load').addEventListener('click', function () {
      var xml = $('xml').value.trim();
      if (!xml) { setStatus($('loadStatus'), 'Bitte DMN-XML einfügen.', 'err'); return; }
      setStatus($('loadStatus'), 'Lade …', null);
      fetch('/v1/models', {
        method: 'POST',
        headers: authHeaders({ 'Content-Type': 'application/xml' }),
        body: xml
      }).then(function (resp) {
        return resp.json().then(function (body) { return { resp: resp, body: body }; });
      }).then(function (r) {
        if (!r.resp.ok) { setStatus($('loadStatus'), errorText(r.resp, r.body), 'err'); return; }
        modelId = r.body.modelId;
        fillIndex(r.body);
        setStatus($('loadStatus'), 'Geladen — ' + (r.body.decisions || []).length + ' Decision(s).', 'ok');
      }).catch(function (e) { setStatus($('loadStatus'), 'Fehler: ' + e, 'err'); });
    });

    function fillIndex(idx) {
      var sel = $('decision');
      sel.innerHTML = '';
      (idx.decisions || []).forEach(function (d) {
        var o = document.createElement('option'); o.value = d; o.textContent = d; sel.appendChild(o);
      });
      $('eval').disabled = (idx.decisions || []).length === 0;

      var pills = $('inputPills'); pills.innerHTML = '';
      (idx.inputs || []).forEach(function (name) {
        var s = document.createElement('span'); s.className = 'pill'; s.textContent = name; pills.appendChild(s);
      });
      $('indexBox').style.display = (idx.inputs || []).length ? 'block' : 'none';

      var tb = $('inputTable').querySelector('tbody'); tb.innerHTML = '';
      (idx.inputs || []).forEach(function (name) {
        var tr = document.createElement('tr');
        var k = document.createElement('td'); k.className = 'kv-key'; k.textContent = name;
        var vtd = document.createElement('td');
        var inp = document.createElement('input'); inp.dataset.key = name; inp.placeholder = 'Wert';
        vtd.appendChild(inp); tr.appendChild(k); tr.appendChild(vtd); tb.appendChild(tr);
      });

      var diags = (idx.diagnostics || []);
      if (diags.length) { renderDiags(diags); $('resultBox').style.display = 'block'; }
    }

    // Coerce a single field: try JSON (numbers, booleans, null, arrays, objects),
    // fall back to the raw string so "Winter" stays "Winter" without quotes.
    function coerce(raw) {
      var s = raw.trim();
      if (s === '') { return undefined; }
      try { return JSON.parse(s); } catch (e) { return raw; }
    }

    function collectInput() {
      var raw = $('inputJson').value.trim();
      if (raw) { return JSON.parse(raw); } // may throw → caught by caller
      var input = {};
      var rows = $('inputTable').querySelectorAll('input');
      for (var i = 0; i < rows.length; i++) {
        var v = coerce(rows[i].value);
        if (v !== undefined) { input[rows[i].dataset.key] = v; }
      }
      return input;
    }

    function evaluate(stateless) {
      var decision = $('decision').value;
      if (!stateless && !modelId) { setStatus($('evalStatus'), 'Bitte zuerst Modell laden.', 'err'); return; }
      if (!decision) { setStatus($('evalStatus'), 'Bitte eine Decision wählen.', 'err'); return; }
      var input;
      try { input = collectInput(); }
      catch (e) { setStatus($('evalStatus'), 'Ungültiges Eingabe-JSON: ' + e.message, 'err'); return; }

      var url, payload;
      if (stateless) {
        url = '/v1/evaluate';
        payload = { xml: $('xml').value, decision: decision, input: input };
      } else {
        url = '/v1/models/' + encodeURIComponent(modelId) + '/evaluate';
        payload = { decision: decision, input: input };
      }
      setStatus($('evalStatus'), 'Werte aus …', null);
      fetch(url, {
        method: 'POST',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify(payload)
      }).then(function (resp) {
        return resp.json().then(function (body) { return { resp: resp, body: body }; });
      }).then(function (r) {
        if (!r.resp.ok) { setStatus($('evalStatus'), errorText(r.resp, r.body), 'err'); return; }
        renderResult(r.body);
        setStatus($('evalStatus'), 'OK', 'ok');
      }).catch(function (e) { setStatus($('evalStatus'), 'Fehler: ' + e, 'err'); });
    }

    $('eval').addEventListener('click', function () { evaluate(false); });
    $('evalStateless').addEventListener('click', function () { evaluate(true); });

    function fmt(v) {
      if (v === null) { return 'null'; }
      if (typeof v === 'object') { return JSON.stringify(v); }
      return String(v);
    }

    function renderResult(res) {
      $('resultBox').style.display = 'block';
      $('rawResult').textContent = JSON.stringify(res, null, 2);
      var tb = $('outTable').querySelector('tbody'); tb.innerHTML = '';
      var outs = res.outputs || {};
      var keys = Object.keys(outs);
      if (!keys.length) {
        var tr = document.createElement('tr');
        var td = document.createElement('td'); td.colSpan = 2; td.className = 'muted';
        td.textContent = 'Keine Outputs.'; tr.appendChild(td); tb.appendChild(tr);
      }
      keys.forEach(function (k) {
        var tr = document.createElement('tr');
        var kt = document.createElement('td'); kt.textContent = k;
        var vt = document.createElement('td'); vt.textContent = fmt(outs[k]);
        tr.appendChild(kt); tr.appendChild(vt); tb.appendChild(tr);
      });
      renderDiags(res.diagnostics || []);
    }

    function renderDiags(diags) {
      var box = $('diags'); box.innerHTML = '';
      diags.forEach(function (d) {
        var sev = (d.severity || 'info').toLowerCase();
        var el = document.createElement('div'); el.className = 'diag ' + sev;
        var where = d.line ? (' (Zeile ' + d.line + (d.col ? ':' + d.col : '') + ')') : '';
        el.textContent = '[' + sev + '] ' + (d.code ? d.code + ' — ' : '') + (d.message || '') + where;
        box.appendChild(el);
      });
    }
  </script>
</body>
</html>
`

// handleUI serves the interactive DMN playground. Like the docs page it is
// always public so the engine is explorable even when the data endpoints require
// a token (the page lets the user supply that token).
func (s *Server) handleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(playgroundPage))
}
