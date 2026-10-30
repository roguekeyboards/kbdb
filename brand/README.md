# brand

> **Moved.** The brand now lives in [mykeebs-brand](https://github.com/roguekeyboards/mykeebs-brand), which publishes `@roguekeyboards/mykeebs-brand`. Make brand changes there. This copy only serves kbdb's consent page, until [#632](https://github.com/roguekeyboards/kbdb/issues/632) removes it.

mykeebs' logo, icons and design tokens. They live in kbdb because the brand belongs to the whole system, not one client: kbdb's sign-in and sign-out page uses them, and so does mykeebs-web.

- **Source of truth:** `tokens.json` (colours and font stacks) and the designs in `internal/art`.
- **Generated:** everything in `dist/`, by `mise run gen`. Never edit `dist/` by hand.
- **Go consumers** (kbdb's pages) import `github.com/roguekeyboards/kbdb/brand`: `brand.Files` holds `dist/`, `brand.Tokens` holds `tokens.json`. Embed the logo inline rather than linking to the web app, so the pages don't depend on it being up.
- **mykeebs-web** installs `@roguekeyboards/mykeebs-brand` from GitHub Packages, published by that repo. Renovate bumps it like the API client.

## Files

| File | Use |
| --- | --- |
| `logo-horizontal-on-dark.svg` / `-on-light.svg` | Mark beside the wordmark: headers, nav bars |
| `logo-stacked-on-dark.svg` / `-on-light.svg` | Mark above the wordmark: square spaces, social cards |
| `mark-on-dark.svg` / `-on-light.svg` | The mark alone |
| `favicon.svg`, `favicon.ico` (16/32/48), `favicon-16.png`, `favicon-32.png` | Browser tab icons, drawn on the pixel grid at 16 and 32 |
| `apple-touch-icon.png` (180) | iOS home screen |
| `icon-192.png`, `icon-512.png` | Web app manifest |
| `discord-avatar.png` (1024) | Prod Discord application icon. Discord crops it to a circle; the mark fits inside. |
| `discord-avatar-dev.png` (1024) | Dev Discord application icon: the same with a blue "DEV" banner |
| `tokens.css` | `tokens.json` as CSS custom properties: theme tokens as `--accent` etc., logo colours as `--logo-amber` etc. |

In the SVGs the wordmark's text is converted to outlines, so they render the same without JetBrains Mono installed.

### Using it in mykeebs-web

```ts
import favicon from '@roguekeyboards/mykeebs-brand/favicon.svg';
```

```css
@import '@roguekeyboards/mykeebs-brand/tokens.css';
```

## Changing the brand

1. Edit `tokens.json` or a design in `internal/art`.
2. `mise run gen`, look at `dist/`, commit both.
3. The PR needs a `feat` or `fix` title so merging cuts a release, which publishes the package and rebuilds kbdb's pages.

`check-generated` compares `dist/`'s SVG and CSS files byte for byte. The PNGs and ICO can differ by a shade on a pixel or two between CPU architectures, so `internal/assets`' tests compare those pixel by pixel within a small tolerance instead.

### By hand, after a release that changes the icons

These don't update themselves:

- [ ] Discord prod application icon (Developer Portal → General Information): `discord-avatar.png`
- [ ] Discord dev application icon: `discord-avatar-dev.png`
- [ ] Stytch email template logo, if one is set

## License

The name, logo and artwork aren't covered by kbdb's MIT license; see [`LICENSE`](LICENSE). JetBrains Mono is under the SIL Open Font License ([`fonts/OFL.txt`](fonts/OFL.txt)).
