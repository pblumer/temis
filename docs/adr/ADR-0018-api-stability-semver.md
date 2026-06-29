# ADR-0018: API-Stabilität, SemVer und Deprecation-Policy für `package dmn`

- **Status:** accepted
- **Datum:** 2026-06-29
- **Kontext-WP:** WP-43 (löst die in ADR-0011 zugesagte SemVer-Disziplin ein)

## Kontext

ADR-0011 hat `package dmn` zum Primärartefakt mit „echter SemVer-Disziplin"
erklärt und als Folgeaufgabe die explizite Stabilitätszusage offengelassen.
ADR-0011 nennt die Kosten ausdrücklich: „Die Lib-API ist jetzt eine harte
Breaking-Change-Grenze." Bis WP-43 war die Oberfläche „experimental". WP-43
friert sie ein. Offene Fragen: **was** genau ist der Vertrag, **wie** wird er
erzwungen (nicht nur dokumentiert), und **wie** werden Änderungen klassifiziert.

Eine reine Prosa-Zusage verrottet: Niemand merkt, wenn ein Refactor versehentlich
ein exportiertes Feld umbenennt oder eine Signatur ändert. Die Zusage muss
maschinell durchgesetzt werden, sonst ist sie wertlos.

## Optionen

1. **Nur Dokumentation.** Stabilitätsversprechen in `40-api-contract.md`, Disziplin
   per Review. — Billig, aber nicht durchgesetzt; versehentliche Brüche bleiben
   unbemerkt bis ein Nutzer sich beschwert.
2. **Externes `apidiff`/`go/packages`-Werkzeug** gegen eine gespeicherte Baseline. —
   Präzise (kennt Typkompatibilität), aber zieht Toolchain/Abhängigkeiten herein,
   was der Anti-Dependency-Linie (ADR-0014) widerspricht.
3. **Golden-Test der exportierten Oberfläche mit der Standardbibliothek**
   (diese Entscheidung). Ein Test parst `package dmn` mit `go/ast`, trimmt via
   `ast.FileExports` auf das Exportierte (inkl. Wegschneiden unexportierter
   Struct-Felder) und vergleicht die gerenderten Deklarationen gegen
   `testdata/api/dmn.api`. — Null neue Abhängigkeiten; jede Änderung der
   Oberfläche bricht CI und erzwingt eine bewusste Golden-Aktualisierung.

## Entscheidung

Option 3, plus die folgende Policy.

**Vertrag (v1).** Der SemVer-stabile Vertrag ist die exportierte Oberfläche von
`package dmn` (siehe `40-api-contract.md §4` für die Symbol-Liste). `internal/`
ist ausdrücklich **nicht** Teil des Vertrags und jederzeit änderbar (ADR-0011).
Zusätzlich gehören zum öffentlichen Verhalten: die Compile-/Eval-Fehlergrenze
(§1.4) und die `Diagnostic.Code`/RFC-7807-`code`-Werte (additiv stabil).

**Durchsetzung.** `dmn/apisurface_test.go` friert die Oberfläche in
`testdata/api/dmn.api` ein. Drift bricht CI. Eine beabsichtigte Änderung wird mit
`go test ./dmn -run TestPublicAPISurface -update-api` aufgenommen — der Golden-Diff
im Review macht die Vertragänderung **sichtbar und bewusst**.

**Klassifizierung.**
- *Additiv* (neues Symbol/Feld/`Code*`-Konstante) → **Minor**.
- *Breaking* (Umbenennen/Entfernen/Signaturänderung; Verschieben der
  Fehlergrenze; Umbenennen eines stabilen `Code`) → **Major**; Go-Modulpfad endet
  dann auf `/vN`.

**Deprecation.** Ein zu entfernendes Symbol wird zuerst mit
`// Deprecated: <Grund + Ersatz>` markiert und bleibt voll funktionsfähig; die
Entfernung erfolgt frühestens im nächsten Major.

## Konsequenzen

**Positiv**
- Die Stabilitätszusage aus ADR-0011 ist jetzt **durchgesetzt**, nicht nur
  versprochen; versehentliche Brüche scheitern in CI.
- Null neue Abhängigkeiten (konsistent mit ADR-0014).
- Der Golden-Diff dokumentiert jede Vertragänderung nachvollziehbar in der Historie.

**Negativ / Kosten**
- Der Golden-Test kennt keine Typkompatibilität (er ist textuell): Auch ein rein
  additives, kompatibles `-update-api` erzeugt einen Diff — die Major-vs-Minor-
  Entscheidung bleibt menschliches Urteil im Review, nicht automatisch.
- Kommentare/Formatierung fließen nicht ein (der Test parst ohne Kommentare und
  rendert nur Signaturen), damit Doku-Edits den Vertrag nicht „brechen".