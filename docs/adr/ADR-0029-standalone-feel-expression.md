# ADR-0029: Öffentliches FEEL-Ausdruck-Primitive (`dmn.CompileExpression`) für volle Flow-Mappings

- **Status:** accepted
- **Datum:** 2026-07-01
- **Kontext-WP:** WP-95 (volle FEEL-Mappings); löst die in ADR-0026 vermerkte Folgeaufgabe, additive v1-Surface (ADR-0019)

## Kontext

ADR-0026 (Decision-Flow-Deskriptor) hat die Step-Input-Mappings zunächst **referenzbasiert**
umgesetzt (WP-90): ein `in`-Wert war entweder ein Flow-Input-Name oder `stepID.output`. Die
**vollen FEEL-Mapping-Ausdrücke** (`"Credit Score - 50"`, `if … then … else …`, Built-ins)
wurden dort explizit als Folgeaufgabe zurückgestellt, mit der Begründung: sie brauchen ein
**öffentliches FEEL-Eval-Primitive in `package dmn`**, damit `flow` FEEL nutzen kann, **ohne
`internal/feel` zu importieren** (Architektur D5/ADR-0005: Adapter erreichen die Engine nur
über die öffentliche `dmn`-API).

Dieses ADR liefert das Primitive und schaltet damit volle FEEL-Mappings frei.

## Optionen

1. **Ein Standalone-FEEL-Primitive zu `package dmn` hinzufügen** (diese Entscheidung).
   `CompileExpression` kompiliert einen FEEL-Ausdruck gegen eine Menge deklarierter Namen;
   `CompiledExpression.Evaluate` wertet ihn gegen einen Namenskontext aus. `flow` konsumiert
   es wie jeder andere Client. — Kosten: die als v1 eingefrorene `dmn`-Surface wächst
   **additiv** (Minor, ADR-0019; Golden-Surface-Test neu erzeugt).

2. **`flow` importiert `internal/feel` direkt.** — Bricht die Paketgrenze (ADR-0005/0011):
   `internal/` ist privat und ohne Stabilitätszusage; ein Nicht-Engine-Paket darf es nicht
   einbinden. Verworfen.

3. **Mappings über synthetische DMN-Modelle auswerten** (je Mapping eine Literal-Expression-
   Decision bauen und `Evaluate` rufen). — Schwergewichtig (XML-Bau, Compile-Overhead pro
   Mapping), umständlich, verfehlt das „compile once"-Modell (ADR-0004). Verworfen.

## Entscheidung

**Option 1.** Neue, additive Symbole in `package dmn`:

```go
type CompiledExpression struct { /* opaque */ }

// Kompiliert einen FEEL-Ausdruck, der die genannten Variablennamen referenzieren darf.
// Ein nicht deklarierter, aber benutzter Name ist ein Compile-Fehler ("unknown variable").
func CompileExpression(expr string, names ...string) (*CompiledExpression, error)

// Die Teilmenge der deklarierten Namen, die der Ausdruck tatsächlich referenziert (sortiert).
func (c *CompiledExpression) References() []string

// Wertet den Ausdruck gegen in (Name → Go-Wert) aus; Rückgabe Go-konvertiert.
func (c *CompiledExpression) Evaluate(ctx context.Context, in Input) (any, error)
```

Intern baut es aus `names` ein `feel.Env`, kompiliert über die bestehende FEEL-Maschinerie
(`internal/feel.CompileStringRefs`) und nutzt `dmn`s vorhandene `inputToValues`/`fromValue`
für die Go⇄FEEL-Konvertierung. **Default-Config**: volle Built-in-Bibliothek (ADR-0003),
Decimal-Zahlen (ADR-0007). `now()`/`today()` lesen die Prozess-Uhr und sind hier **nicht**
deterministisch — für deterministische Zeit einen Fixwert als Input übergeben.

**`References()`** kommt aus einer minimalen, **allokationsfreien** Erweiterung des
FEEL-Compilers: `CompileStringRefs` erfasst, welche Namen des Root-`Env` der Ausdruck
auflöst. Der normale Compile-Pfad bleibt unberührt (Tracking-Felder nil).

### Flow-Mappings (WP-95)

`flow` behandelt jeden `in`- und `output`-Wert nun so:

- **Plain-Referenz (Fast Path, rückwärtskompatibel):** `stepID.output` oder **exakt ein
  deklarierter Flow-Input-Name** → wie WP-90 direkt aufgelöst (mit Typ-Coercion).
- **Sonst: voller FEEL-Ausdruck**, kompiliert gegen `{deklarierte Inputs} ∪ {Step-IDs}`.
  Der Auswertungskontext ist: alle Flow-Inputs plus jeder frühere Step-Output als Kontext
  unter der Step-ID (`risk` → `{"Risk Level": …}`).

**DAG-Ordnung bleibt erhalten:** die Abhängigkeiten eines Steps sind `References() ∩ Step-IDs`
— ein FEEL-Ausdruck, der einen Step nennt, erzeugt dieselbe Kante wie eine Plain-Referenz.
Kompiliert wird **compile-before-eval**: ein ungültiger Ausdruck (Syntaxfehler, unbekannte
Variable) ist eine `FLOW_MAPPING_INVALID`-Diagnostic vor jeder Auswertung.

## Konsequenzen

**Positiv**
- Mappings können Flow-Inputs beliebig transformieren (Arithmetik, `if`, String-/Listen-/
  Datums-Built-ins, Vergleiche) — nicht nur durchreichen.
- `dmn.CompileExpression` ist ein eigenständig nützliches Primitive (FEEL als kleine
  Ausdruckssprache über einen Namenskontext), nicht flow-spezifisch.
- `flow` bleibt frei von `internal/`-Importen; die DAG-Ordnung und Rückwärtskompatibilität
  der Plain-Referenzen bleiben erhalten.

**Negativ / Kosten**
- Die `dmn`-v1-Surface wächst (additiv, Golden neu erzeugt) und ist ab jetzt SemVer-gebunden.
- **Numerik über Step-Grenzen:** Step-Outputs stehen im FEEL-Kontext als Dezimal-**Strings**
  (dmn rendert Zahlen so, ADR-0007). Direkte FEEL-Arithmetik auf einer *numerischen
  Step-Ausgabe* (`risk.Score + 10`) ergibt daher `null`; Durchreichen an ein number-Ziel
  funktioniert weiter (Post-Eval-Coercion), und Transformationen auf **Flow-Inputs** (typisiert
  vom Aufrufer) funktionieren voll. Für Cross-Step-Arithmetik einen Decision-Step nutzen.
- **Mehrwort-Member-Zugriff:** FEEL-Pfadzugriff `risk.Risk Level` parst den Mehrwort-Member
  nicht; solche Step-Outputs über die Plain-Referenz `"stepID.Risk Level"` oder
  `get value(risk, "Risk Level")` ansprechen.
- **Deklarierte Inputs nötig:** um einen Input in einem *Ausdruck* zu referenzieren, muss er
  in `inputs` deklariert sein (sonst „unknown variable"). Plain-Referenzen auf undeklarierte
  Inputs entfallen damit — eine geringfügige Verschärfung gegenüber WP-90 (alle mitgelieferten
  Beispiele deklarieren ihre Inputs).

**Folgeaufgaben**
- `docs/40-api-contract.md` um `CompileExpression`/`CompiledExpression` ergänzt.
- ADR-0026-Folgeaufgabe „Mapping-Ausbau" ist damit erledigt.
