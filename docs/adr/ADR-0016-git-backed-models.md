# ADR-0016: DMN-Modelle aus einem Git-Repository lesen und (später) bearbeiten

- **Status:** proposed
- **Datum:** 2026-06-29
- **Kontext-WP:** WP-60 (Lesen/Browsen), WP-61 (Schreiben/Commit/Branch/PR)

## Kontext

Bisher werden Modelle ad hoc hochgeladen (`POST /v1/models`, MCP `load_model`) und
inhaltsadressiert (`sha256:`) im Speicher gecacht. Es fehlt eine **Herkunft**: Modelle
leben nicht versioniert, ein Modell „auf Branch *release*" oder „im Commit *abc123*" ist
kein erstklassiger Begriff. Gewünscht ist, dass Temis DMN-Dateien **aus einem
Git-Repository liest und bearbeitet** und dabei den ganzen Git-Workflow (Branch, Commit,
Merge/PR) mitnimmt — als SaaS zunächst über **GitHub**, grundsätzlich aber mit **jedem
Git-Remote**.

Das berührt zwei bisherige **Nicht-Ziele des MVP** (`00-overview.md` §3): „kein
eingebauter Persistenzlayer" und „kein Decision-Management/Versionierung". Diese galten
für das MVP; mit erreichtem MVP und laufender Beta ist eine **versionierte Modellquelle**
eine bewusste, eigenständige Erweiterung — daher dieses ADR.

Zwei Anforderungen stehen in Spannung:

1. **„Mit jedem Git-Remote"** verlangt eine generische Git-Mechanik (clone/fetch/commit/
   merge über HTTPS/SSH), unabhängig vom Hoster.
2. **„SaaS-Start mit GitHub"** und die Projektkultur — **genau eine** externe Dependency
   (`apd/v3`), eigener FEEL-Stack (ADR-0004), eigener HTTP-Mux, MCP ohne SDK (ADR-0014),
   **Goldene Regel 6**: neue Dependency = ADR + Begründung — sprechen gegen einen großen
   neuen Abhängigkeitsbaum zum Einstieg.

## Optionen

1. **`go-git` sofort** (`github.com/go-git/go-git`). Reines Go, clone/commit/branch/merge
   über HTTPS/SSH gegen **jeden** Remote, in-process, kein lokales `git`-Binary. — Bricht
   die 1-Dependency-Linie deutlich (großer transitiver Baum), und ein voller Server-seitiger
   Clone pro Tenant/Repo ist im SaaS-Multitenancy schwergewichtig (Platz, Lebenszyklus).
2. **`git`-CLI per `exec`.** Universell, stdlib-nah (nur `os/exec`). — Braucht ein
   installiertes `git`-Binary **und** einen lokalen Arbeitsbaum pro Tenant; Prozess-/
   Dateisystem-Isolation und Credential-Handling sind im SaaS heikel.
3. **Provider-Interface, GitHub-API zuerst** (diese Entscheidung). Ein schmales
   `vcs.Reader`-Interface (später `vcs.Writer`) abstrahiert die Git-Mechanik; erste konkrete
   Implementierung ist **GitHub REST** über die **Standardbibliothek** (`net/http`,
   `encoding/json`). — Erfüllt „SaaS-Start mit GitHub" und „jeder Remote" (über das
   Interface) und kostet **null neue Dependencies**. Kosten: pro Hoster ein Adapter; echte
   beliebige Remotes (SSH, selbst-gehostet ohne GitHub-kompatible API) kommen erst mit einem
   weiteren Backend (dann `go-git` oder CLI — als eigenes Folge-ADR).

## Entscheidung (Empfehlung)

**Option 3.** Ein Paket `vcs` definiert das Provider-Interface und die Engine-Anbindung;
`vcs/github` implementiert es über die GitHub-REST-API mit reiner Standardbibliothek. Die
Wahl der generischen Git-Mechanik (`go-git` vs. CLS) wird damit **bewusst vertagt**, nicht
still getroffen — sie wird ein eigenes ADR, sobald ein nicht-GitHub-Remote oder echtes
Server-seitiges Merging konkret gefordert ist.

Begründung: Die Abstraktion erfüllt beide Anforderungen gleichzeitig, ohne die
Dependency-Kultur des Projekts zu brechen, und hält die teure, schwer umkehrbare
Entscheidung (großer Git-Stack, Server-Clones) offen, bis ihr konkreter Nutzen feststeht.
Das ist dieselbe Linie wie ADR-0014 (MCP ohne SDK): den Standard sprechen, ohne den
großen Baukasten aufzunehmen, solange der schmale Scope ihn nicht rechtfertigt.

> Status `proposed`: Die endgültige Mechanik-Wahl liegt beim Maintainer (offene
> Architekturfrage gem. `60-ai-agent-guide.md` §5). WP-60 (Lesen/Browsen) implementiert
> bereits den `vcs.Reader` + GitHub-Adapter; das ist unter jeder der drei Optionen
> nützlich und unter Option 3 vollständig.

## Surface (geplant)

- **Library (Kern, library-first):** `package vcs` mit `Reader`-Interface (`ListBranches`,
  `ListCommits`, `ListFiles`, `ReadFile`) und `Models` (bindet `Reader` + `dmn.Engine`:
  DMN aus einem Ref laden → kompilieren). `vcs/github.Client` als erster Provider. Schreiben
  (`Writer`: `Commit`, `CreateBranch`, `OpenPullRequest`) folgt in WP-61.
- **Wrapper (Folge-WPs, dünn über `vcs`):** `temisd`-Endpunkte (Repos/Branches/Dateien
  lesen, später committen), MCP-Tools (Repo durchsuchen, Modell aus Branch laden/auswerten),
  optional Git-Browser in `/ui`. Diese hängen **nur** von `vcs` und `dmn` ab — kein
  `internal/`-Zugriff (ADR-0005).

## Konsequenzen

**Positiv**
- Keine neue Abhängigkeit; das Projekt bleibt bei einer einzigen (`apd/v3`, Goldene Regel 6).
- „Jeder Remote" bleibt durch das Interface erreichbar; weitere Backends ohne Caller-Änderung.
- Versionierte, reproduzierbare Modellquelle (Ref = Branch/Tag/Commit) als erstklassiger
  Begriff über alle Oberflächen.

**Negativ / Kosten**
- Pro Hoster ein Adapter; echte beliebige Remotes (SSH, Nicht-GitHub-API) erst mit einem
  weiteren Backend.
- Server-seitiges 3-Wege-/XML-Merging ist mit der reinen API nicht möglich — Merge wird dem
  Provider überlassen (GitHub-PR-Merge). Falls Temis selbst mergen soll, braucht es Option 1/2.
- Credential-/Auth-Modell (Token-Herkunft, Multitenancy) ist in den Wrapper-WPs noch zu
  entscheiden.

**Revisit-Trigger**
Ein neues ADR ersetzt/ergänzt dieses, sobald **eines** gilt: (a) ein nicht-GitHub-Remote
oder SSH-Zugriff wird gefordert; (b) Temis soll **selbst** mergen statt über den Provider.
Dann wird `go-git` (Option 1) oder die CLI (Option 2) als weiteres Backend hinter `vcs`
aufgenommen — mit Dependency-/Multitenancy-Begründung.
