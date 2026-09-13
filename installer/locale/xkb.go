package locale

import (
	"strings"
	"unicode"
)

// xkbAliases covers the console keymaps whose names do not simply shorten to
// their XKB layout. Console keymaps come from kbd and XKB layouts from
// xkeyboard-config; the two naming schemes agree often enough that a rule
// handles most cases, but not for these.
var xkbAliases = map[string]string{
	"uk":          "gb",
	"dvorak":      "us",
	"dvorak-l":    "us",
	"dvorak-r":    "us",
	"colemak":     "us",
	"emacs":       "us",
	"emacs2":      "us",
	"defkeymap":   "us",
	"sg":          "ch", // Swiss German
	"sf":          "ch", // Swiss French
	"fr_CH":       "ch",
	"de_CH":       "ch",
	"la-latin1":   "latam",
	"trq":         "tr",
	"trf":         "tr",
	"jp106":       "jp",
	"nb":          "no",
	"no-latin1":   "no",
	"dk-latin1":   "dk",
	"br-abnt":     "br",
	"br-abnt2":    "br",
	"pt-latin1":   "pt",
	"es-cp850":    "es",
	"cf":          "ca", // Canadian French
	"ruwin_alt":   "ru",
	"ttwin_alt":   "ru",
	"croat":       "hr",
	"slovene":     "si",
	"bg_bds-utf8": "bg",
	"bg_pho-utf8": "bg",
	"ua-utf":      "ua",
	"gr":          "gr", // kbd's "gr" is Greek: its charset is iso-8859-7
}

// xkbSuffixes are console-keymap decorations that have no bearing on the XKB
// layout name; the closest XKB equivalent would be a variant, which Hyprland's
// kb_variant handles separately and which is not worth prompting for.
var xkbSuffixes = []string{
	"-latin1", "-latin9", "-latin0", "-nodeadkeys", "-deadkeys", "-dead",
	"-qwertz", "-qwerty", "-abnt2", "-abnt", "-cp850", "-utf8", "-utf",
	"-dvorak", "-colemak", "-bksl", "-ps", "-olpc",
}

// XKBLayout maps a console keymap name onto the XKB layout Hyprland's
// kb_layout wants.
//
// The two vocabularies only mostly overlap, so this is a best effort with a
// safe floor: anything unrecognisable falls back to "us", which leaves a
// usable keyboard rather than a broken one. The console keymap itself — the
// one that has to be right for typing a LUKS passphrase at boot — is written
// verbatim to /etc/vconsole.conf and never goes through this.
func XKBLayout(keymap string) string {
	if keymap == "" {
		return "us"
	}
	if layout, ok := xkbAliases[keymap]; ok {
		return layout
	}

	name := strings.ToLower(keymap)
	if layout, ok := xkbAliases[name]; ok {
		return layout
	}

	// Apple layouts are named mac-<layout> or <layout>-mac.
	name = strings.TrimPrefix(name, "mac-")
	name = strings.TrimSuffix(name, "-mac")

	for _, suffix := range xkbSuffixes {
		name = strings.TrimSuffix(name, suffix)
	}
	if layout, ok := xkbAliases[name]; ok {
		return layout
	}

	// Whatever is left before the first separator is the layout for the great
	// majority of keymaps: "fr-pc" -> "fr", "cz_qwerty" -> "cz".
	if i := strings.IndexAny(name, "-_."); i > 0 {
		name = name[:i]
	}
	if layout, ok := xkbAliases[name]; ok {
		return layout
	}

	// XKB layout names are two- or three-letter codes; anything else means the
	// rules above did not actually resolve to a layout.
	if isAlpha(name) && (len(name) == 2 || len(name) == 3) {
		return name
	}
	return "us"
}

func isAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return s != ""
}
