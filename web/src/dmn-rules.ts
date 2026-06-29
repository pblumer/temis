import RuleProvider from 'diagram-js/lib/features/rules/RuleProvider'
import type EventBus from 'diagram-js/lib/core/EventBus'

// Seed of the DMN modeling rules (ADR-0016, WP-65). For now it simply permits
// moving elements so the viewer becomes editable; real DRG rules (which
// requirement may connect which elements, no cycles, …) build on this.
class DmnRules extends RuleProvider {
  static $inject = ['eventBus']

  constructor(eventBus: EventBus) {
    super(eventBus)
  }

  init(): void {
    this.addRule('shape.move', () => true)
    this.addRule('elements.move', () => true)
    // Permit the context-pad connect action; real DRG connection rules (which
    // requirement is legal between which elements) refine this later.
    this.addRule('connection.start', () => true)
    this.addRule('connection.create', () => true)
    this.addRule('connection.reconnect', () => true)
  }
}

export const dmnRulesModule = {
  __init__: ['dmnRules'],
  dmnRules: ['type', DmnRules],
}
