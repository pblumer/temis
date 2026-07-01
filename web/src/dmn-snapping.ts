import CreateMoveSnapping from 'diagram-js/lib/features/snapping/CreateMoveSnapping'
import ResizeSnapping from 'diagram-js/lib/features/snapping/ResizeSnapping'
import Snapping from 'diagram-js/lib/features/snapping/Snapping'
import { append as svgAppend, attr as svgAttr, classes as svgClasses, create as svgCreate } from 'tiny-svg'

// Alignment guides while dragging (the "Hilfslinien" feature): diagram-js' own
// element-to-element snapping already lines a moved node up with the others —
// aligning its centre and its edges to their centres — and draws a guide line
// where they meet. We reuse that logic wholesale and only change how the lines
// appear: instead of hard-switching the SVG `display` attribute (which cannot be
// animated), each line stays in the DOM and we toggle a `--active` class so the
// stylesheet can ease its opacity in and out for the soft fade the design asks
// for (see .djs-snap-line in style.css). The guides align nodes to each other,
// so the background dot grid is decorative only and not involved here.

// How far each guide line reaches past the shapes, so it spans the whole canvas.
const GUIDE_SPAN = 100000

type Orientation = 'horizontal' | 'vertical'
type GuideLine = { update: (position?: number | false) => void }

class AlignSnapping extends Snapping {
  static $inject = ['canvas']

  // Overrides Snapping#_createLine: same geometry as upstream, but visibility is
  // driven by the `djs-snap-line--active` class (opacity) rather than `display`,
  // so the CSS transition can fade the guide in when it snaps and out when it
  // lets go. A non-numeric position means "no snap on this axis" → fade out.
  _createLine(orientation: Orientation): GuideLine {
    const root = (this as unknown as { _canvas: { getLayer: (name: string) => SVGElement } })._canvas.getLayer('snap')
    const line = svgCreate('path')
    svgAttr(line, { d: 'M0,0 L0,0' })
    svgClasses(line).add('djs-snap-line')
    svgAppend(root, line)

    return {
      update(position) {
        if (typeof position !== 'number') {
          svgClasses(line).remove('djs-snap-line--active')
          return
        }
        const d =
          orientation === 'horizontal'
            ? `M-${GUIDE_SPAN},${position} L+${GUIDE_SPAN},${position}`
            : `M${position},-${GUIDE_SPAN} L${position},+${GUIDE_SPAN}`
        svgAttr(line, { d })
        svgClasses(line).add('djs-snap-line--active')
      },
    }
  }
}

export const dmnSnappingModule = {
  __init__: ['createMoveSnapping', 'resizeSnapping', 'snapping'],
  createMoveSnapping: ['type', CreateMoveSnapping],
  resizeSnapping: ['type', ResizeSnapping],
  snapping: ['type', AlignSnapping],
}
