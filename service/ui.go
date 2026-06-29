package service

import "net/http"

// playgroundPage is the interactive DMN UI served at "/" and "/ui": a single
// HTML document with inline CSS and vanilla JavaScript. It embeds bpmn.io's
// dmn-js (loaded from the jsDelivr CDN, like the Swagger UI at /docs) so a model
// can be *viewed*, *edited* and *deployed* straight from the browser:
//
//   - Upload a .dmn file (or paste DMN-XML) -> rendered read-only in dmn-js.
//   - Toggle "Bearbeiten" -> the same model opens in the editable dmn-js Modeler.
//   - "Auf Server deployen" -> POST /v1/models (compile + cache), which surfaces
//     the model's decisions and inputs in the evaluation panel.
//   - Evaluate a chosen decision (POST /v1/models/{id}/evaluate) or run a
//     stateless evaluation against the current XML (POST /v1/evaluate).
//
// An optional bearer token is sent as Authorization on the gated /v1 calls.
//
// dmn-js is embedded UNCHANGED (CDN UMD bundles, bpmn.io logo intact) — see
// ADR-0006/ADR-0012. The read-only viewer and the editable modeler ship as two
// bundles that both export the global window.DmnJS, so they are loaded
// sequentially and each global is captured right after its script loads.
//
// The script avoids JS template literals on purpose: the page lives in a Go raw
// string literal, so it must contain no backticks.
const playgroundPage = `<!DOCTYPE html>
<html lang="de">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Temis — DMN Editor</title>
  <link rel="icon" href="data:,">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/dmn-js@17.8.1/dist/assets/diagram-js.css">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/dmn-js@17.8.1/dist/assets/dmn-js-shared.css">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/dmn-js@17.8.1/dist/assets/dmn-js-drd.css">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/dmn-js@17.8.1/dist/assets/dmn-js-decision-table.css">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/dmn-js@17.8.1/dist/assets/dmn-js-decision-table-controls.css">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/dmn-js@17.8.1/dist/assets/dmn-js-literal-expression.css">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/dmn-js@17.8.1/dist/assets/dmn-font/css/dmn.css">
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
    main { max-width: 1600px; margin: 0 auto; padding: 24px;
      display: grid; grid-template-columns: 230px 1.4fr 1fr; gap: 24px; }
    @media (max-width: 1100px) { main { grid-template-columns: 1fr; } }
    /* VS Code-style explorer for models stored on the server. */
    .explorer { background: var(--panel); border: 1px solid var(--border);
      border-radius: 10px; padding: 12px; align-self: start; }
    .explorer .head { display: flex; align-items: center; justify-content: space-between;
      text-transform: uppercase; letter-spacing: .04em; color: var(--muted);
      font-size: 12px; font-weight: 600; margin-bottom: 8px; }
    .explorer .head button { background: transparent; color: var(--accent); border: none;
      padding: 2px 6px; font-size: 13px; cursor: pointer; }
    .tree { font-family: var(--mono); font-size: 12.5px; }
    .tree .empty { color: var(--muted); font-family: inherit; font-size: 13px; }
    .tree .model > .label { display: flex; align-items: center; gap: 6px; padding: 3px 4px;
      border-radius: 4px; cursor: pointer; color: var(--fg); white-space: nowrap;
      overflow: hidden; text-overflow: ellipsis; }
    .tree .model > .label:hover { background: #20283a; }
    .tree .model.active > .label { background: #1f6feb33; color: #cfe0ff; }
    .tree .model .chev { color: var(--muted); width: 10px; display: inline-block; }
    .tree .model .ico { color: var(--accent); }
    .tree .decs { margin: 0 0 4px 18px; }
    .tree .decs .dec { color: var(--muted); padding: 2px 4px; white-space: nowrap;
      overflow: hidden; text-overflow: ellipsis; }
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
    textarea { resize: vertical; min-height: 120px; }
    button {
      background: var(--accent); color: #fff; border: none; border-radius: 6px;
      padding: 9px 16px; font-size: 14px; font-weight: 600; cursor: pointer;
    }
    button.secondary { background: transparent; color: var(--accent);
      border: 1px solid var(--border); }
    button:disabled { opacity: .5; cursor: not-allowed; }
    .row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-top: 12px; }
    .grow { flex: 1; }
    /* dmn-js renders on a light surface; keep its own theming intact. */
    #dmnCanvas { height: 460px; background: #fff; border: 1px solid var(--border);
      border-radius: 8px; position: relative; overflow: hidden; }
    #dmnHint { position: absolute; inset: 0; display: flex; align-items: center;
      justify-content: center; color: #6b7280; font-size: 14px; text-align: center;
      padding: 24px; pointer-events: none; }
    .badge { display: inline-block; font-size: 12px; font-weight: 600;
      border-radius: 999px; padding: 3px 10px; border: 1px solid var(--border); }
    .badge.read { color: var(--muted); }
    .badge.write { color: var(--ok); border-color: var(--ok); }
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
    /* Evaluation overlay: a value badge under each traversed decision node, plus
       a coloured node outline (blue = requested decision, green = intermediate). */
    .temis-badge { font: 600 11px/1.3 var(--mono); color: #fff; padding: 2px 7px;
      border-radius: 6px; white-space: nowrap; max-width: 240px; overflow: hidden;
      text-overflow: ellipsis; box-shadow: 0 1px 4px rgba(0,0,0,.35); background: #2da44e; }
    .temis-badge.final { background: #1f6feb; }
    .djs-element.temis-eval .djs-visual > :first-child { stroke: #2da44e !important; stroke-width: 3px !important; }
    .djs-element.temis-final .djs-visual > :first-child { stroke: #1f6feb !important; stroke-width: 3px !important; }
    /* Per-decision variable scope (expandable): the input data and upstream
       decisions a decision can see, with the values they held at evaluation. */
    .scope-item { border: 1px solid var(--border); border-radius: 6px; margin: 6px 0; padding: 6px 10px; }
    .scope-item > summary { cursor: pointer; display: flex; justify-content: space-between; gap: 12px; }
    .scope-item .dec { font-family: var(--mono); color: var(--accent); }
    .scope-item .dec.final { color: #4f8bff; font-weight: 600; }
    .scope-item .val { font-family: var(--mono); }
    .scope-vars { margin-top: 8px; border-top: 1px solid var(--border); padding-top: 6px; }
    .scope-vars .row2 { display: flex; justify-content: space-between; gap: 12px;
      font-family: var(--mono); font-size: 12.5px; padding: 2px 0; }
    .scope-vars .vk { color: var(--muted); }
  </style>
</head>
<body>
  <header>
    <h1>Temis — DMN Editor</h1>
    <span class="sub">Modell hochladen · ansehen · bearbeiten · deployen · auswerten</span>
    <a href="/docs">API-Doku (Swagger UI) →</a>
  </header>
  <main>
    <aside class="explorer">
      <div class="head">
        <span>Server-Modelle</span>
        <button id="refreshModels" title="Liste aktualisieren">⟳</button>
      </div>
      <div id="modelTree" class="tree"><div class="empty">— wird geladen —</div></div>
    </aside>

    <section class="panel">
      <h2>1 · Modell</h2>
      <div class="row">
        <button id="newFile" class="secondary">Neu</button>
        <input type="file" id="file" accept=".dmn,.xml,application/xml,text/xml" class="grow">
        <span id="modeBadge" class="badge read">schreibgeschützt</span>
        <button id="modeToggle" class="secondary" disabled>Bearbeiten</button>
      </div>
      <label for="modelName">Modellname</label>
      <input type="text" id="modelName" placeholder="z. B. Rabattlogik — benennt das Modell (statt des Felds im Diagramm)">
      <div id="dmnCanvas">
        <div id="dmnHint">Lade dmn-js …</div>
      </div>
      <label for="token">Bearer-Token (optional)</label>
      <input type="text" id="token" placeholder="nur nötig, wenn der Server -token verlangt">
      <div class="row">
        <button id="deploy" disabled>Auf Server deployen</button>
        <span id="loadStatus" class="status"></span>
      </div>
      <div id="indexBox" style="display:none">
        <label>Erkannte Inputs</label>
        <div id="inputPills" class="muted"></div>
      </div>
      <details>
        <summary>Alternativ: DMN-XML einfügen</summary>
        <textarea id="xml" spellcheck="false" placeholder="<definitions ...> … </definitions>"></textarea>
        <div class="row">
          <button id="importXml" class="secondary">In Editor laden</button>
        </div>
      </details>
    </section>

    <section class="panel">
      <h2>2 · Auswerten</h2>
      <p class="muted" id="evalHint">Erst „Auf Server deployen", dann Decision wählen und auswerten.</p>
      <label for="decision">Decision</label>
      <select id="decision"><option value="">— erst deployen —</option></select>
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
        <button id="evalStateless" class="secondary" disabled>Stateless auswerten</button>
        <span id="evalStatus" class="status"></span>
      </div>
      <div id="resultBox" style="display:none">
        <h2 style="margin-top:20px">Ergebnis</h2>
        <p class="muted" id="resultIntro">Durchlaufene Decisions — links im Diagramm markiert (★ = angefragte). Aufklappen (oder Knoten anklicken) zeigt den Variablen-Scope:</p>
        <div id="decisionList"></div>
        <div id="diags"></div>
        <details>
          <summary>Rohe Antwort</summary>
          <pre id="rawResult"></pre>
        </details>
      </div>
    </section>
  </main>

  <script>
    var CDN = 'https://cdn.jsdelivr.net/npm/dmn-js@17.8.1/dist/';
    // Minimal blank DMN 1.3 model (one decision with an empty decision table)
    // used by the "Neu" button to start from scratch. Kept on one line so the
    // surrounding Go raw string needs no escaping.
    var BLANK_DMN = '<?xml version="1.0" encoding="UTF-8"?>' +
      '<definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/" ' +
      'xmlns:dmndi="https://www.omg.org/spec/DMN/20191111/DMNDI/" ' +
      'xmlns:dc="http://www.omg.org/spec/DMN/20180521/DC/" ' +
      'id="definitions_new" name="neues_Modell" namespace="http://temis/new">' +
      '<decision id="Decision_1" name="Entscheidung_1">' +
      '<decisionTable id="DecisionTable_1" hitPolicy="UNIQUE">' +
      '<output id="Output_1" label="Ausgabe" name="Ausgabe" typeRef="string"/>' +
      '</decisionTable></decision>' +
      '<dmndi:DMNDI><dmndi:DMNDiagram id="DMNDiagram_1">' +
      '<dmndi:DMNShape id="DMNShape_Decision_1" dmnElementRef="Decision_1">' +
      '<dc:Bounds height="80" width="180" x="160" y="100"/>' +
      '</dmndi:DMNShape></dmndi:DMNDiagram></dmndi:DMNDI></definitions>';
    var DmnViewer = null, DmnModeler = null; // captured from window.DmnJS per bundle
    var dmn = null;                           // current dmn-js instance
    var mode = 'read';                        // 'read' | 'write'
    var loaded = false;                       // a diagram is currently shown
    var lastXML = '';                         // last known model XML
    var modelId = null;                       // set after a successful deploy
    var annotatedIds = [];                    // DRD element ids currently annotated

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

    function loadScript(src) {
      return new Promise(function (resolve, reject) {
        var s = document.createElement('script');
        s.src = src; s.crossOrigin = 'anonymous';
        s.onload = resolve;
        s.onerror = function () { reject(new Error('Konnte ' + src + ' nicht laden')); };
        document.head.appendChild(s);
      });
    }

    // Both dmn-js bundles export the same global (window.DmnJS); load them in
    // sequence and grab each global immediately after its script runs.
    loadScript(CDN + 'dmn-navigated-viewer.production.min.js').then(function () {
      DmnViewer = window.DmnJS;
      return loadScript(CDN + 'dmn-modeler.production.min.js');
    }).then(function () {
      DmnModeler = window.DmnJS;
      $('dmnHint').textContent = 'Datei hochladen, leeres Modell anlegen („Neu") oder links ein Server-Modell wählen.';
    }).catch(function (e) {
      $('dmnHint').textContent = 'dmn-js konnte nicht geladen werden: ' + e.message;
    });

    // --- server model explorer (left sidebar) ---

    // Short, readable id suffix for models that declare no name.
    function shortId(id) {
      var hex = String(id).replace(/^sha256:/, '');
      return 'modell-' + hex.slice(0, 8);
    }

    function modelLabel(m) {
      return (m.name && m.name.trim()) ? m.name.trim() : shortId(m.modelId);
    }

    // GET /v1/models and render the tree; called on load and after each deploy.
    function refreshModels() {
      fetch('/v1/models', { headers: authHeaders({}) }).then(function (resp) {
        if (!resp.ok) { return { models: [] }; } // listing disabled or error → empty
        return resp.json();
      }).then(function (body) {
        renderModelTree((body && body.models) || []);
      }).catch(function () { renderModelTree([]); });
    }

    function renderModelTree(models) {
      var tree = $('modelTree');
      tree.innerHTML = '';
      if (!models.length) {
        var e = document.createElement('div'); e.className = 'empty';
        e.textContent = 'Noch keine Modelle deployed.'; tree.appendChild(e); return;
      }
      models.forEach(function (m) {
        var wrap = document.createElement('div'); wrap.className = 'model'; wrap.dataset.id = m.modelId;
        if (m.modelId === modelId) { wrap.className += ' active'; }

        var label = document.createElement('div'); label.className = 'label';
        var chev = document.createElement('span'); chev.className = 'chev'; chev.textContent = '▸';
        var ico = document.createElement('span'); ico.className = 'ico'; ico.textContent = '◧';
        var nm = document.createElement('span'); nm.textContent = modelLabel(m) + '.dmn';
        nm.title = modelLabel(m) + '  (' + m.modelId + ')';
        label.appendChild(chev); label.appendChild(ico); label.appendChild(nm);
        wrap.appendChild(label);

        var decs = document.createElement('div'); decs.className = 'decs'; decs.style.display = 'none';
        (m.decisions || []).forEach(function (d) {
          var row = document.createElement('div'); row.className = 'dec'; row.textContent = '› ' + d; row.title = d;
          decs.appendChild(row);
        });
        wrap.appendChild(decs);

        // Click the chevron to expand decisions; click the name to open the model.
        chev.addEventListener('click', function (ev) {
          ev.stopPropagation();
          var open = decs.style.display !== 'none';
          decs.style.display = open ? 'none' : 'block';
          chev.textContent = open ? '▸' : '▾';
        });
        nm.addEventListener('click', function () { loadServerModel(m.modelId); });
        tree.appendChild(wrap);
      });
    }

    // Open a server-stored model in the editor: fetch its XML, render it
    // read-only, and wire up evaluation against the already-cached model.
    function loadServerModel(id) {
      setStatus($('loadStatus'), 'Lade Server-Modell …', null);
      fetch('/v1/models/' + encodeURIComponent(id) + '/xml', { headers: authHeaders({}) })
        .then(function (resp) { if (!resp.ok) { throw new Error('HTTP ' + resp.status); } return resp.text(); })
        .then(function (xml) {
          $('xml').value = xml; $('file').value = '';
          return renderDiagram(xml, 'read');
        })
        .then(function () {
          resetDeployState();
          // It is already on the server — fetch its index so evaluation works
          // immediately without re-deploying, and mark it active in the tree.
          modelId = id;
          return fetch('/v1/models/' + encodeURIComponent(id), { headers: authHeaders({}) });
        })
        .then(function (resp) { return resp.ok ? resp.json() : null; })
        .then(function (idx) {
          if (idx) { fillIndex(idx); }
          markActiveModel();
          setStatus($('loadStatus'), 'Server-Modell geladen (schreibgeschützt).', 'ok');
        })
        .catch(function (e) { setStatus($('loadStatus'), 'Fehler: ' + (e.message || e), 'err'); });
    }

    function markActiveModel() {
      var items = $('modelTree').querySelectorAll('.model');
      for (var i = 0; i < items.length; i++) {
        var on = items[i].dataset.id === modelId;
        items[i].className = on ? 'model active' : 'model';
      }
    }

    $('refreshModels').addEventListener('click', refreshModels);

    function updateModeUI() {
      var badge = $('modeBadge');
      if (mode === 'write') {
        badge.textContent = 'bearbeitbar'; badge.className = 'badge write';
        $('modeToggle').textContent = 'Schreibgeschützt';
      } else {
        badge.textContent = 'schreibgeschützt'; badge.className = 'badge read';
        $('modeToggle').textContent = 'Bearbeiten';
      }
      $('modeToggle').disabled = !loaded || !DmnViewer || !DmnModeler;
      $('deploy').disabled = !loaded;
    }

    // (Re)create the dmn-js instance for the requested mode and import the XML.
    function renderDiagram(xml, newMode) {
      return new Promise(function (resolve, reject) {
        if (dmn) { try { dmn.destroy(); } catch (e) { /* ignore */ } dmn = null; }
        annotatedIds = []; // overlays/markers are gone with the destroyed instance
        $('dmnHint').style.display = 'none';
        var Ctor = (newMode === 'write') ? DmnModeler : DmnViewer;
        dmn = new Ctor({ container: '#dmnCanvas' });
        dmn.importXML(xml).then(function () {
          var v = dmn.getActiveViewer && dmn.getActiveViewer();
          try { if (v && v.get('canvas')) { v.get('canvas').zoom('fit-viewport'); } } catch (e) { /* ignore */ }
          mode = newMode; lastXML = xml; loaded = true; updateModeUI();
          syncModelNameField();
          resolve();
        }).catch(reject);
      });
    }

    // The DMN definitions moddle element, or null. dmn-js exposes it on both the
    // viewer and the modeler.
    function getDefs() {
      if (!dmn || !dmn.getDefinitions) { return null; }
      try { return dmn.getDefinitions(); } catch (e) { return null; }
    }

    // Mirror the model's name into the comfortable "Modellname" field. (The
    // in-canvas dmn-js name label is awkward to edit, so this is the place to
    // rename a model.)
    function syncModelNameField() {
      var d = getDefs();
      $('modelName').value = (d && d.name) ? d.name : '';
    }

    // Write the "Modellname" field back onto the model before it is serialized,
    // so the name travels with the DMN XML (deploy, download, server list).
    function applyModelName() {
      var d = getDefs();
      if (!d) { return; }
      d.name = $('modelName').value.trim();
    }

    // Resolve to the current model XML, serialized from the live editor so the
    // model name (and any edits) are included; falls back to the last import.
    function getCurrentXML() {
      applyModelName();
      if (dmn && dmn.saveXML) {
        return dmn.saveXML({ format: true })
          .then(function (r) { return r.xml; })
          .catch(function () { return lastXML || ''; });
      }
      return Promise.resolve(lastXML || '');
    }

    // A freshly loaded model invalidates any previous deploy/evaluation: drop the
    // model id and clear the evaluation panel so stale decisions/results vanish.
    function resetDeployState() {
      modelId = null;
      var sel = $('decision');
      sel.innerHTML = '<option value="">— erst deployen —</option>';
      $('eval').disabled = true;
      $('evalStateless').disabled = true;
      $('evalHint').style.display = 'block';
      $('inputTable').querySelector('tbody').innerHTML = '';
      $('inputPills').innerHTML = '';
      $('indexBox').style.display = 'none';
      $('resultBox').style.display = 'none';
      $('evalStatus').textContent = '';
    }

    function loadFromText(text) {
      if (!DmnViewer) { setStatus($('loadStatus'), 'dmn-js lädt noch …', 'err'); return; }
      renderDiagram(text, 'read').then(function () {
        resetDeployState();
        setStatus($('loadStatus'), 'Geladen (schreibgeschützt). Zum Ändern „Bearbeiten".', 'ok');
      }).catch(function (e) {
        setStatus($('loadStatus'), 'Kein gültiges DMN: ' + (e.message || e), 'err');
      });
    }

    $('newFile').addEventListener('click', function () {
      if (!DmnModeler) { setStatus($('loadStatus'), 'dmn-js lädt noch …', 'err'); return; }
      $('file').value = '';
      $('xml').value = '';
      renderDiagram(BLANK_DMN, 'write').then(function () {
        resetDeployState();
        setStatus($('loadStatus'), 'Neues, leeres Modell — bearbeiten und dann deployen.', 'ok');
      }).catch(function (e) {
        setStatus($('loadStatus'), 'Fehler: ' + (e.message || e), 'err');
      });
    });

    $('file').addEventListener('change', function (ev) {
      var f = ev.target.files[0];
      if (!f) { return; }
      var rd = new FileReader();
      rd.onload = function () { $('xml').value = String(rd.result); loadFromText(String(rd.result)); };
      rd.readAsText(f);
    });

    $('importXml').addEventListener('click', function () {
      var xml = $('xml').value.trim();
      if (!xml) { setStatus($('loadStatus'), 'Bitte DMN-XML einfügen.', 'err'); return; }
      loadFromText(xml);
    });

    $('modeToggle').addEventListener('click', function () {
      if (!loaded) { return; }
      if (mode === 'write') {
        // Persist edits, then drop back to the read-only viewer.
        getCurrentXML().then(function (xml) { return renderDiagram(xml, 'read'); })
          .then(function () { setStatus($('loadStatus'), 'Schreibgeschützt.', 'ok'); })
          .catch(function (e) { setStatus($('loadStatus'), 'Fehler: ' + (e.message || e), 'err'); });
      } else {
        renderDiagram(lastXML, 'write')
          .then(function () { setStatus($('loadStatus'), 'Bearbeitbar — Änderungen dann deployen.', 'ok'); })
          .catch(function (e) { setStatus($('loadStatus'), 'Fehler: ' + (e.message || e), 'err'); });
      }
    });

    function asJson(resp) { return resp.json().then(function (body) { return { resp: resp, body: body }; }); }

    $('deploy').addEventListener('click', function () {
      if (!loaded) { setStatus($('loadStatus'), 'Bitte zuerst ein Modell laden.', 'err'); return; }
      setStatus($('loadStatus'), 'Speichere & deploye …', null);
      getCurrentXML().then(function (xml) {
        lastXML = xml;
        return fetch('/v1/models', {
          method: 'POST',
          headers: authHeaders({ 'Content-Type': 'application/xml' }),
          body: xml
        });
      }).then(asJson).then(function (r) {
        if (!r.resp.ok) { setStatus($('loadStatus'), errorText(r.resp, r.body), 'err'); return; }
        modelId = r.body.modelId;
        if (r.body.name) { $('modelName').value = r.body.name; }
        fillIndex(r.body);
        setStatus($('loadStatus'), 'Deployed — ' + (r.body.decisions || []).length + ' Decision(s).', 'ok');
        refreshModels(); // the new model now appears in the server list
      }).catch(function (e) { setStatus($('loadStatus'), 'Fehler: ' + (e.message || e), 'err'); });
    });

    function fillIndex(idx) {
      var sel = $('decision');
      sel.innerHTML = '';
      (idx.decisions || []).forEach(function (d) {
        var o = document.createElement('option'); o.value = d; o.textContent = d; sel.appendChild(o);
      });
      var has = (idx.decisions || []).length > 0;
      $('eval').disabled = !has;
      $('evalStateless').disabled = !has;
      $('evalHint').style.display = has ? 'none' : 'block';

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
      if (!stateless && !modelId) { setStatus($('evalStatus'), 'Bitte zuerst deployen.', 'err'); return; }
      if (!decision) { setStatus($('evalStatus'), 'Bitte eine Decision wählen.', 'err'); return; }
      var input;
      try { input = collectInput(); }
      catch (e) { setStatus($('evalStatus'), 'Ungültiges Eingabe-JSON: ' + e.message, 'err'); return; }

      setStatus($('evalStatus'), 'Werte aus …', null);
      getCurrentXML().then(function (xml) {
        var url, payload;
        if (stateless) {
          url = '/v1/evaluate';
          payload = { xml: xml, decision: decision, input: input, explain: true };
        } else {
          url = '/v1/models/' + encodeURIComponent(modelId) + '/evaluate';
          payload = { decision: decision, input: input, explain: true };
        }
        return fetch(url, {
          method: 'POST',
          headers: authHeaders({ 'Content-Type': 'application/json' }),
          body: JSON.stringify(payload)
        });
      }).then(asJson).then(function (r) {
        if (!r.resp.ok) {
          // Hide any prior result and clear the graph so a stale success isn't
          // shown next to the error.
          $('resultBox').style.display = 'none';
          clearAnnotations();
          setStatus($('evalStatus'), errorText(r.resp, r.body), 'err');
          return;
        }
        renderResult(r.body, decision, input);
        setStatus($('evalStatus'), 'OK', 'ok');
      }).catch(function (e) { setStatus($('evalStatus'), 'Fehler: ' + (e.message || e), 'err'); });
    }

    $('eval').addEventListener('click', function () { evaluate(false); });
    $('evalStateless').addEventListener('click', function () { evaluate(true); });

    function fmt(v) {
      if (v === null) { return 'null'; }
      if (typeof v === 'object') { return JSON.stringify(v); }
      return String(v);
    }

    function renderResult(res, finalDecision, evalInput) {
      $('resultBox').style.display = 'block';
      $('rawResult').textContent = JSON.stringify(res, null, 2);
      renderDiags(res.diagnostics || []);
      // One DRD pass: annotate the graph, render the per-decision scope (which
      // needs the DRD connections) and wire node clicks to the scope list.
      withDrd(function (viewer) {
        var reg = null;
        if (viewer) { try { reg = viewer.get('elementRegistry'); } catch (e) { /* ignore */ } }
        annotateOn(viewer, res, finalDecision);
        renderScope(reg, res, finalDecision, evalInput);
        bindNodeClick(viewer);
      });
    }

    // Run cb with the DRD view's viewer, switching to the DRD view first if a
    // decision-table/literal view is currently open. Falls back to the active
    // viewer when the views API is unavailable.
    function withDrd(cb) {
      if (!dmn || !dmn.getActiveViewer) { cb(null); return; }
      var av = dmn.getActiveView && dmn.getActiveView();
      if (av && av.type === 'drd') { cb(dmn.getActiveViewer()); return; }
      var views = (dmn.getViews && dmn.getViews()) || [];
      var drd = views.filter(function (v) { return v.type === 'drd'; })[0];
      if (!drd || !dmn.open) { cb(dmn.getActiveViewer()); return; }
      Promise.resolve(dmn.open(drd)).then(function () { cb(dmn.getActiveViewer()); }).catch(function () { /* ignore */ });
    }

    // Remove all evaluation markers/badges from the DRD.
    function clearAnnotations() {
      withDrd(function (viewer) {
        if (!viewer) { return; }
        try {
          var canvas = viewer.get('canvas'), overlays = viewer.get('overlays');
          annotatedIds.forEach(function (id) {
            canvas.removeMarker(id, 'temis-eval'); canvas.removeMarker(id, 'temis-final');
          });
          overlays.clear();
        } catch (e) { /* ignore */ }
        annotatedIds = [];
      });
    }

    // Find the DRD element for a decision by its name (or id).
    function decisionElement(reg, name) {
      if (!reg) { return null; }
      return reg.filter(function (e) {
        var bo = e.businessObject;
        return bo && bo.$type === 'dmn:Decision' && (bo.name === name || e.id === name);
      })[0] || null;
    }

    // Annotate the DRD: outline every traversed decision and badge it with the
    // value it produced; the requested ("final") decision is highlighted apart.
    function annotateOn(viewer, res, finalDecision) {
      if (!viewer) { return; }
      var reg, overlays, canvas;
      try { reg = viewer.get('elementRegistry'); overlays = viewer.get('overlays'); canvas = viewer.get('canvas'); }
      catch (e) { return; }
      annotatedIds.forEach(function (id) {
        try { canvas.removeMarker(id, 'temis-eval'); canvas.removeMarker(id, 'temis-final'); } catch (e) { /* ignore */ }
      });
      try { overlays.clear(); } catch (e) { /* ignore */ }
      annotatedIds = [];

      var decisions = res.decisions || {};
      Object.keys(decisions).forEach(function (name) {
        var el = decisionElement(reg, name);
        if (!el) { return; }
        var isFinal = (name === finalDecision);
        try { canvas.addMarker(el.id, isFinal ? 'temis-final' : 'temis-eval'); } catch (e) { /* ignore */ }
        annotatedIds.push(el.id);

        var full = fmt(decisions[name]);
        var text = (full.length > 40) ? (full.slice(0, 39) + '…') : full;
        var div = document.createElement('div');
        div.className = 'temis-badge' + (isFinal ? ' final' : '');
        div.title = name + ' = ' + full;
        div.textContent = text;
        try { overlays.add(el.id, { position: { bottom: -8, left: 0 }, html: div }); } catch (e) { /* ignore */ }
      });
    }

    // depsOf returns a decision's direct scope: the input data and upstream
    // decisions it requires, read from the DRD's incoming connections.
    function depsOf(reg, name) {
      var deps = [];
      var el = decisionElement(reg, name);
      if (!el || !el.incoming) { return deps; }
      el.incoming.forEach(function (conn) {
        var src = conn.source;
        if (!src || !src.businessObject) { return; }
        var t = src.businessObject.$type, nm = src.businessObject.name || src.id;
        if (t === 'dmn:InputData') { deps.push({ name: nm, kind: 'input' }); }
        else if (t === 'dmn:Decision') { deps.push({ name: nm, kind: 'decision' }); }
      });
      return deps;
    }

    function scopeRow(label, value) {
      var r = document.createElement('div'); r.className = 'row2';
      var a = document.createElement('span'); a.className = 'vk'; a.textContent = label;
      var b = document.createElement('span'); b.textContent = value;
      r.appendChild(a); r.appendChild(b);
      return r;
    }

    // Render the traversed decisions as an expandable list; each entry shows the
    // variable scope that decision saw (its inputs + upstream decision values).
    function renderScope(reg, res, finalDecision, evalInput) {
      var box = $('decisionList'); box.innerHTML = '';
      var decisions = res.decisions || {};
      var keys = Object.keys(decisions);
      if (!keys.length) {
        var p = document.createElement('p'); p.className = 'muted';
        p.textContent = 'Keine Decisions ausgewertet.'; box.appendChild(p); return;
      }
      keys.forEach(function (name) {
        var det = document.createElement('details'); det.className = 'scope-item'; det.dataset.name = name;
        var sum = document.createElement('summary');
        var isFinal = (name === finalDecision);
        var dec = document.createElement('span'); dec.className = 'dec' + (isFinal ? ' final' : '');
        dec.textContent = isFinal ? (name + ' ★') : name;
        var val = document.createElement('span'); val.className = 'val'; val.textContent = fmt(decisions[name]);
        sum.appendChild(dec); sum.appendChild(val); det.appendChild(sum);

        var vars = document.createElement('div'); vars.className = 'scope-vars';
        var deps = depsOf(reg, name);
        if (!deps.length) {
          var m = document.createElement('div'); m.className = 'muted';
          m.textContent = 'Keine eingehenden Variablen.'; vars.appendChild(m);
        } else {
          deps.forEach(function (d) {
            var v;
            if (d.kind === 'input') {
              v = (evalInput && (d.name in evalInput)) ? fmt(evalInput[d.name]) : '—';
            } else {
              v = (d.name in decisions) ? fmt(decisions[d.name]) : '—';
            }
            vars.appendChild(scopeRow(d.name + (d.kind === 'input' ? ' (Eingabe)' : ' (Decision)'), v));
          });
        }
        det.appendChild(vars); box.appendChild(det);
      });
    }

    // Clicking a decision node opens (and scrolls to) its scope entry. Bound once
    // per viewer instance; a fresh diagram re-binds on the next evaluation.
    function bindNodeClick(viewer) {
      if (!viewer || viewer.__temisClickBound) { return; }
      var eb;
      try { eb = viewer.get('eventBus'); } catch (e) { return; }
      eb.on('element.click', function (ev) {
        var el = ev.element;
        if (!el || !el.businessObject || el.businessObject.$type !== 'dmn:Decision') { return; }
        var name = el.businessObject.name || el.id;
        var items = document.querySelectorAll('#decisionList .scope-item');
        for (var i = 0; i < items.length; i++) {
          if (items[i].dataset.name === name) { items[i].open = true; items[i].scrollIntoView({ block: 'nearest' }); }
        }
      });
      viewer.__temisClickBound = true;
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

    // Populate the server-model explorer on first load.
    refreshModels();
  </script>
</body>
</html>
`

// handleUI serves the interactive DMN editor. Like the docs page it is always
// public so the engine is explorable even when the data endpoints require a
// token (the page lets the user supply that token).
func (s *Server) handleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(playgroundPage))
}
