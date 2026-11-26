package tui

import "github.com/charmbracelet/bubbles/key"

func bind(label, help string, keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(help, label))
}

// The keys, declared once: the footer and the keybindings overlay both read
// these, so they never disagree. Their order in each list is their priority.
var (
	keyQuit     = bind("Quit", "q", "q", "ctrl+c")
	keyHelp     = bind("Keybindings", "?", "?")
	keyRefresh  = bind("Refresh", "r", "r")
	keyNew      = bind("New", "n", "n")
	keyStop     = bind("Stop", "x", "x")
	keyAccount  = bind("Account", "a", "a")
	keyFocus    = bind("Focus", "tab", "tab")
	keyViews    = viewKeys()
	keyMove     = bind("Move", "↑/↓", "up", "down", "k", "j")
	keyTabs     = bind("Tabs", "[ ]", "[", "]")
	keyDay      = bind("Day", "←/→", "left", "right", "h", "l")
	keyWeek     = bind("Week", "←/→", "left", "right", "h", "l")
	keyToday    = bind("Today", "t", "t")
	keyContinue = bind("Continue", "s", "s")
	keyEdit     = bind("Edit", "enter", "enter", "e")
	keyDelete   = bind("Delete", "d", "d")
	keySave     = bind("Save", "enter", "enter")
	keyUse      = bind("Use", "enter", "enter")
	keyRange    = bind("Range", "t", "t")
	keyOpenDay  = bind("Open day", "enter", "enter")
	keyStart    = bind("Start", "enter", "enter")
	keyFilter   = bind("Filter", "/", "/")
	keyBack     = bind("Back", "esc", "esc")
	keyNext     = bind("Next field", "tab", "tab")
)

// viewKeys jumps to a view by its number, one per view: the keybindings help
// lists each number with its view, the footer does not.
func viewKeys() key.Binding {
	numbers := make([]string, len(viewNames))
	for i := range viewNames {
		numbers[i] = string(rune('1' + i))
	}
	return key.NewBinding(key.WithKeys(numbers...))
}

// globalKeys are the keys that work everywhere with no overlay open; focus
// only means something on the main screen.
func (m model) globalKeys() []key.Binding {
	if m.screen == accountScreen {
		return []key.Binding{keyNew, keyStop, keyRefresh, keyQuit, keyHelp}
	}
	return []key.Binding{keyNew, keyStop, keyAccount, keyFocus, keyRefresh, keyQuit, keyHelp}
}

// viewKeys are the keys of what is on screen, most useful first.
func (m model) viewKeys() []key.Binding {
	switch {
	case m.overlay == formOverlay && m.form.editing != nil:
		return []key.Binding{keySave, keyNext, keyMove, keyBack}
	case m.overlay == formOverlay && m.form.suggest >= 0:
		return []key.Binding{keyUse, keyMove, keyNext, keyBack}
	case m.overlay == formOverlay:
		return []key.Binding{keyStart, keyNext, keyMove, keyBack}
	case m.overlay == helpOverlay:
		return []key.Binding{keyBack}
	case m.filtering:
		return []key.Binding{keyStart, keyBack}
	case m.screen == accountScreen:
		return []key.Binding{keyBack}
	case m.focus == sidebarFocus:
		return []key.Binding{keyMove, keyFocus}
	case m.view == projectsView:
		return []key.Binding{keyMove, keyStart, keyFilter, keyFocus}
	case m.view == reportsView && m.reportTab == balanceTab:
		return []key.Binding{keyTabs, keyFocus}
	case m.view == reportsView:
		return []key.Binding{keyMove, keyRange, keyTabs, keyFocus}
	case m.tab == weekTab:
		return []key.Binding{keyMove, keyWeek, keyOpenDay, keyToday, keyTabs}
	default:
		return []key.Binding{keyMove, keyDay, keyEdit, keyContinue, keyDelete, keyToday, keyTabs}
	}
}
