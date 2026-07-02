# Third-party notices — Temis Modeler frontend

The built frontend (`web/dist/`, embedded into the `temisd` binary via `go:embed`)
bundles the following third-party packages. All are permissive (MIT / ISC /
Apache-2.0) — none carries the bpmn.io license or a logo/attribution-in-UI
requirement. This is the MIT core of the bpmn-io toolkit (diagram-js and its
runtime dependencies); the bpmn.io-licensed **dmn-js** wrapper is deliberately
**not** used (ADR-0016). Alongside it, **@dagrejs/dagre** (with its transitive
**@dagrejs/graphlib**) supplies the layered auto-layout for the read-only flow
views — ranks, crossing-minimization and edge waypoints in one pass.

| Package | Version | License |
|---|---|---|
| diagram-js | 15.18.0 | MIT |
| @bpmn-io/diagram-js-ui | 0.2.4 | MIT |
| min-dom | 5.3.0 | MIT |
| min-dash | 5.0.0 | MIT |
| tiny-svg | 4.1.4 | MIT |
| didi | 11.0.0 | MIT |
| object-refs | 0.4.0 | MIT |
| path-intersection | 4.1.0 | MIT |
| domify | 3.0.0 | MIT |
| clsx | 2.1.1 | MIT |
| preact | 10.29.3 | MIT |
| htm | 3.1.1 | Apache-2.0 |
| inherits-browser | 0.1.0 | ISC |
| @dagrejs/dagre | 3.0.0 | MIT |
| @dagrejs/graphlib | 4.0.1 | MIT |

The full license texts ship with each package under `web/node_modules/<pkg>/LICENSE`.
The MIT License text below applies to the MIT-licensed packages above (copyright
held by their respective authors, e.g. diagram-js © 2014-present Camunda Services GmbH).

```
The MIT License (MIT)

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
the Software, and to permit persons to whom the Software is furnished to do so,
subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```
