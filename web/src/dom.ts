// dom.ts — shared, security-critical DOM helpers.
//
// escapeHtml is the single canonical HTML escaper. It escapes the five
// characters that are unsafe in *both* element text and quoted attribute
// contexts, so a value like a FEEL string literal ("Winter") or a
// server-supplied descriptor/type name cannot break out of a value="…" /
// title="…" attribute — closing an earlier finding where a partial escaper
// (missing the quote characters) allowed attribute-injection / stored XSS when
// its output was interpolated into attributes (audit findings H1/H2).
export function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}
