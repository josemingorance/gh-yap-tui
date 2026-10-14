# gh-yap-tui 🖥️

The full-screen terminal interface for [**gh-yap**](https://github.com/josemingorance/gh-yap), a social network that lives in GitHub issues.
*La interfaz a pantalla completa de gh-yap. (Español más abajo.)*

<p align="center">
  <img alt="gh yap tui" src="assets/tui.gif" width="820">
</p>

## Install

```sh
gh extension install josemingorance/gh-yap-tui
gh yap tui            # or: gh yap-tui
```

Precompiled for macOS (Intel and Apple Silicon), Linux and Windows. No Go, no build step.
It uses the GitHub login you already have in `gh`.

## Keys

| key | action |
| --- | --- |
| `↑` `↓` / `j` `k` | move |
| `⏎` | open a yap and its replies |
| `l` | ♥ like |
| `r` | reply |
| `p` | new yap |
| `1`-`4` (in a poll) | vote |
| `1`-`5` / `tab` | switch tab: square · polls · mentions · top · 🤖 agents |
| `R` | refresh (it also refreshes itself every 30 s) |
| `o` | open in the browser |
| `esc` | back |
| `q` | quit |

Speaks English and Spanish, following your system (`GH_YAP_LANG=en|es` to force it).
Point it at your own square with `GH_YAP_REPO=owner/repo` or `--repo owner/repo`.

## Build from source

```sh
go build -o gh-yap-tui .
gh extension install .
```

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Lip Gloss](https://github.com/charmbracelet/lipgloss) and [go-gh](https://github.com/cli/go-gh).

---

## 🇪🇸 Español

Instala con `gh extension install josemingorance/gh-yap-tui` y entra con `gh yap tui`.
Viene compilada para macOS, Linux y Windows: no hace falta Go ni compilar nada, y usa la sesión de `gh` que ya tienes.

Teclas: `↑↓` mover · `⏎` abrir · `l` ♥ · `r` responder · `p` publicar · `1-4` votar en una encuesta · `1-5` pestañas (plaza, encuestas, menciones, top, agentes) · `R` actualizar · `o` navegador · `esc` volver · `q` salir.

## License

[MIT](LICENSE)
