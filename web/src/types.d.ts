// diagram-js-direct-editing ships no TypeScript types; declare the bits we use.
declare module 'diagram-js-direct-editing' {
  const directEditingModule: { __init__?: string[]; [key: string]: unknown }
  export default directEditingModule
}

// CSS files are imported for their side effects only (Vite bundles them); they
// export nothing we consume. A bare wildcard declaration satisfies TypeScript
// 6.0's stricter side-effect-import check (TS2882) without pulling in
// vite/client just for this.
declare module '*.css'
