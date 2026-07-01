# GitHub Social Preview

Das Bild, das GitHub beim Teilen eines Repo-Links zeigt (Teams, Slack, WhatsApp,
X, …). Es ersetzt das generische Repo-Icon durch eine gebrandete Karte.

![Temis Social Preview](social-preview.png)

- Datei: [`social-preview.png`](social-preview.png) — 1280×640, ~150 KB (PNG).
- Motiv: Temis-Logo (Raute/Häkchen) + Wortmarke, Claim und ein `evaluate`-Terminal-Mock.

## Setzen (einmalig, nur über die GitHub-UI)

GitHub bietet **keine API** für das Social-Preview-Bild — es lässt sich nur manuell
hochladen:

1. Repo → **Settings** → **General**.
2. Abschnitt **Social preview**.
3. **Edit** → **Upload an image…** → `docs/social-preview.png` wählen.
4. Speichern.

> Hinweis: Teams/Slack cachen Link-Vorschauen teils tagelang. Erscheint die neue
> Karte nicht sofort, in einem neuen Chat testen oder abwarten.

## Bild neu erzeugen

Die Vorlage lässt sich mit jedem Browser als 1280×640-Screenshot rendern (z. B. aus
einer HTML-Vorlage per Headless-Chromium). GitHub-Vorgaben: PNG/JPG/GIF, empfohlen
1280×640 (Seitenverhältnis 2:1), unter 1 MB.
