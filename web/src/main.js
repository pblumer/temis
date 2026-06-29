import DmnModeler from 'dmn-js/lib/Modeler';

import 'dmn-js/dist/assets/diagram-js.css';
import 'dmn-js/dist/assets/dmn-js-shared.css';
import 'dmn-js/dist/assets/dmn-js-drd.css';
import 'dmn-js/dist/assets/dmn-js-decision-table.css';
import 'dmn-js/dist/assets/dmn-js-decision-table-controls.css';
import 'dmn-js/dist/assets/dmn-js-literal-expression.css';
import 'dmn-js/dist/assets/dmn-font/css/dmn.css';

import './style.css';
import { STARTER_DMN } from './starter.js';
import { loadModel, evaluate } from './api.js';
import { initBranding } from './branding.js';

// Theme/CI anwenden, bevor der Editor rendert (vermeidet Farb-Flackern).
initBranding();

const $ = (id) => document.getElementById(id);

const modeler = new DmnModeler({ container: '#canvas' });

let modelId = null;

async function openDiagram(xml) {
  try {
    await modeler.importXML(xml);
    const active = modeler.getActiveViewer();
    if (active && active.get('canvas').zoom) {
      active.get('canvas').zoom('fit-viewport');
    }
  } catch (err) {
    setStatus($('loadStatus'), 'Diagramm konnte nicht geöffnet werden: ' + err.message, 'err');
  }
}

function setStatus(el, msg, kind) {
  el.textContent = msg;
  el.className = 'status' + (kind ? ' ' + kind : '');
}

// Coerce one input field: try JSON (numbers, booleans, null, arrays, objects),
// fall back to the raw string so "Winter" stays "Winter" without quotes.
function coerce(raw) {
  const s = raw.trim();
  if (s === '') return undefined;
  try {
    return JSON.parse(s);
  } catch {
    return raw;
  }
}

function collectInput() {
  const input = {};
  $('inputTable').querySelectorAll('input').forEach((el) => {
    const v = coerce(el.value);
    if (v !== undefined) input[el.dataset.key] = v;
  });
  return input;
}

function fillIndex(idx) {
  const sel = $('decision');
  sel.innerHTML = '';
  (idx.decisions || []).forEach((d) => {
    const o = document.createElement('option');
    o.value = d;
    o.textContent = d;
    sel.appendChild(o);
  });
  $('eval').disabled = (idx.decisions || []).length === 0;

  const pills = $('inputPills');
  pills.innerHTML = '';
  (idx.inputs || []).forEach((name) => {
    const s = document.createElement('span');
    s.className = 'pill';
    s.textContent = name;
    pills.appendChild(s);
  });
  $('indexBox').hidden = (idx.inputs || []).length === 0;

  const tb = $('inputTable').querySelector('tbody');
  tb.innerHTML = '';
  (idx.inputs || []).forEach((name) => {
    const tr = document.createElement('tr');
    const k = document.createElement('td');
    k.className = 'kv-key';
    k.textContent = name;
    const vtd = document.createElement('td');
    const inp = document.createElement('input');
    inp.dataset.key = name;
    inp.placeholder = 'Wert';
    vtd.appendChild(inp);
    tr.appendChild(k);
    tr.appendChild(vtd);
    tb.appendChild(tr);
  });

  renderDiags(idx.diagnostics || []);
}

function fmt(v) {
  if (v === null) return 'null';
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}

function renderResult(res) {
  $('resultBox').hidden = false;
  $('rawResult').textContent = JSON.stringify(res, null, 2);
  const tb = $('outTable').querySelector('tbody');
  tb.innerHTML = '';
  const outs = res.outputs || {};
  const keys = Object.keys(outs);
  if (keys.length === 0) {
    const tr = document.createElement('tr');
    const td = document.createElement('td');
    td.colSpan = 2;
    td.className = 'muted';
    td.textContent = 'Keine Outputs.';
    tr.appendChild(td);
    tb.appendChild(tr);
  }
  keys.forEach((k) => {
    const tr = document.createElement('tr');
    const kt = document.createElement('td');
    kt.textContent = k;
    const vt = document.createElement('td');
    vt.textContent = fmt(outs[k]);
    tr.appendChild(kt);
    tr.appendChild(vt);
    tb.appendChild(tr);
  });
  renderDiags(res.diagnostics || []);
}

function renderDiags(diags) {
  const box = $('diags');
  box.innerHTML = '';
  diags.forEach((d) => {
    const sev = (d.severity || 'info').toLowerCase();
    const el = document.createElement('div');
    el.className = 'diag ' + sev;
    const where = d.line ? ' (Zeile ' + d.line + (d.col ? ':' + d.col : '') + ')' : '';
    el.textContent = '[' + sev + '] ' + (d.code ? d.code + ' — ' : '') + (d.message || '') + where;
    box.appendChild(el);
  });
}

async function onLoad() {
  setStatus($('loadStatus'), 'Speichere & prüfe …', null);
  try {
    const { xml } = await modeler.saveXML({ format: true });
    const idx = await loadModel(xml, $('token').value.trim());
    modelId = idx.modelId;
    fillIndex(idx);
    setStatus($('loadStatus'), 'Geladen — ' + (idx.decisions || []).length + ' Decision(s).', 'ok');
  } catch (err) {
    setStatus($('loadStatus'), 'Fehler: ' + err.message, 'err');
  }
}

async function onEval() {
  if (!modelId) {
    setStatus($('evalStatus'), 'Bitte zuerst „Modell prüfen".', 'err');
    return;
  }
  const decision = $('decision').value;
  if (!decision) {
    setStatus($('evalStatus'), 'Bitte eine Decision wählen.', 'err');
    return;
  }
  setStatus($('evalStatus'), 'Werte aus …', null);
  try {
    const res = await evaluate(modelId, decision, collectInput(), $('token').value.trim());
    renderResult(res);
    setStatus($('evalStatus'), 'OK', 'ok');
  } catch (err) {
    setStatus($('evalStatus'), 'Fehler: ' + err.message, 'err');
  }
}

$('file').addEventListener('change', (ev) => {
  const f = ev.target.files[0];
  if (!f) return;
  const rd = new FileReader();
  rd.onload = () => openDiagram(String(rd.result));
  rd.readAsText(f);
});
$('load').addEventListener('click', onLoad);
$('eval').addEventListener('click', onEval);

openDiagram(STARTER_DMN);
