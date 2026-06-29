(function(){const r=document.createElement("link").relList;if(r&&r.supports&&r.supports("modulepreload"))return;for(const e of document.querySelectorAll('link[rel="modulepreload"]'))s(e);new MutationObserver(e=>{for(const t of e)if(t.type==="childList")for(const i of t.addedNodes)i.tagName==="LINK"&&i.rel==="modulepreload"&&s(i)}).observe(document,{childList:!0,subtree:!0});function n(e){const t={};return e.integrity&&(t.integrity=e.integrity),e.referrerPolicy&&(t.referrerPolicy=e.referrerPolicy),e.crossOrigin==="use-credentials"?t.credentials="include":e.crossOrigin==="anonymous"?t.credentials="omit":t.credentials="same-origin",t}function s(e){if(e.ep)return;e.ep=!0;const t=n(e);fetch(e.href,t)}})();const d="Temis Modeler",l="WP-60 — Frontend-Toolchain & go:embed";function a(o){var n;const r=document.createElement("p");r.className="probe",r.textContent="OK",o.innerHTML=`
    <main>
      <h1>${d}</h1>
      <p class="sub">Eigener DMN-Modeler · embedded build · kein CDN, offline (ADR-0016)</p>
      <p class="wp">${l}</p>
      <p class="hint">
        Gerüst steht. Hier entsteht der eigene Modeler auf dem geforkten MIT-Kern
        (diagram-js/table-js/dmn-moddle): DRD-Canvas, Decision-Table-Editor und
        FEEL-Validierung gegen die echte temis-Engine.
      </p>
    </main>`,(n=o.querySelector("main"))==null||n.appendChild(r)}const c=document.getElementById("app");c&&a(c);
