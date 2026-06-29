import Diagram from 'diagram-js'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type ElementFactory from 'diagram-js/lib/core/ElementFactory'
import 'diagram-js/assets/diagram-js.css'

// WP-61 proof: render a canvas with the forked MIT core (diagram-js) alone —
// no dmn-js, no bpmn.io logo, no CDN (everything is bundled + embedded). This
// draws a throwaway placeholder graph (a decision fed by an input) just to prove
// the canvas, element factory and default renderer work end-to-end. Real DMN
// shapes (Decision/InputData/BKM/KnowledgeSource + requirement edges) and the
// modeling interactions land on top of this core in WP-65.
export function mountCanvas(container: HTMLElement): void {
  const diagram = new Diagram({ canvas: { container } })
  const canvas = diagram.get<Canvas>('canvas')
  const elementFactory = diagram.get<ElementFactory>('elementFactory')

  const decision = elementFactory.createShape({ id: 'decision', x: 210, y: 80, width: 150, height: 70 })
  canvas.addShape(decision)

  const input = elementFactory.createShape({ id: 'input', x: 235, y: 250, width: 100, height: 50 })
  canvas.addShape(input)

  canvas.addConnection(
    elementFactory.createConnection({
      id: 'requirement',
      source: input,
      target: decision,
      waypoints: [
        { x: 285, y: 250 },
        { x: 285, y: 150 },
      ],
    }),
  )

  canvas.zoom('fit-viewport')
}
