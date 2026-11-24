# TUI design

How `tmetric ui` looks and behaves. Follow it for every screen in `internal/tui`.

## Layout

- The UI takes the whole terminal. Panels stretch to the full width and height, even when they hold little data.
- The main screen is master-detail: a narrow sidebar on the left picks the context, a wide panel on the right shows its detail.
- The right panel can hold tabs. They sit in its top border, separated by `│`. Switch them with `[` and `]`.
- A secondary screen (for example the account) replaces the main screen with stacked boxes. `esc` goes back.
- Overlays (forms, the keybindings help) are centered boxes drawn on top of the screen.

## Boxes

- Every panel is a box with a thin border. Its title sits inside the top border: `┌ Week ──────`.
- The focused panel has an accent-colored border. Other borders use the default color.

## Content

- Tables have no row lines. Headers are uppercase. Columns are left-aligned with generous spacing.
- Columns adapt to the terminal width. The last text column (for example the description) takes the remaining space and is cut with `…`.
- The table cursor colors the whole row with the cursor color, with no background.
- The selected item of a sidebar list gets a highlight background.
- Detail blocks are key-value pairs: the label is dimmed, the value uses the default color.
- A dimmed summary line at the bottom of a panel says what it shows: `2 entries · 5h06m`.
- A dimmed caption at the top of a panel says the range it covers: `week of 2026-11-23`.
- Quantities against a limit use a progress bar with a legend of small colored blocks.

## Footer

- One line at the bottom of the screen lists the keys: `Move: ↑/↓ | Focus: tab | Refresh: r | Quit: q | Keybindings: ?`.
- The label uses the default color; the key uses the accent color.
- The keys change with the focused panel and the open tab: show only the keys that work there.
- Status and error messages replace the footer for a few seconds, then the keys come back.

## Keys

| Key | Action |
|---|---|
| `↑`/`↓` (`k`/`j`) | Move in the focused list |
| `tab` | Move the focus between sidebar and panel |
| `1`…`9` | Jump to a view of the sidebar (listed only in `?`, next to each view's name) |
| `[` / `]` | Previous / next tab |
| `enter` | Open the selected row; it never changes data |
| `n` | New |
| `e` | Edit |
| `d` | Delete (asks for confirmation) |
| `/` | Filter the focused list |
| `t` | Change the time range |
| `r` | Refresh |
| `esc` | Back, or close an overlay |
| `?` | Keybindings help |
| `q` | Quit |

An action that stops or deletes tracked time asks first, in the footer: `Stop "standup" (1h30m)? y/N`.
`y` confirms; any other key cancels. An action that loses nothing (starting with no timer running) does not ask.

## Colors

- Use the terminal's ANSI palette (colors 0–15), never fixed RGB colors, so the UI follows the user's theme.
- Accent (focused borders, active tab, keys in the footer): cyan (6).
- Table cursor: magenta (5).
- Sidebar selection: a background highlight.
- Labels, inactive tabs, captions and summaries: dimmed (faint).
- Errors: red (1).
