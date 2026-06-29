// diagram-js-direct-editing ships no TypeScript types; declare the bits we use.
declare module 'diagram-js-direct-editing' {
  const directEditingModule: { __init__?: string[]; [key: string]: unknown }
  export default directEditingModule
}
