package locale

import "testing"

func TestXKBLayout(t *testing.T) {
	cases := map[string]string{
		// The common case: the console keymap already is the layout name.
		"us": "us",
		"fr": "fr",
		"it": "it",
		"ru": "ru",

		// Decorations that map to an XKB variant, not a layout.
		"de-latin1":            "de",
		"de-latin1-nodeadkeys": "de",
		"fr-latin9":            "fr",
		"cz-qwerty":            "cz",
		"es-cp850":             "es",
		"pt-latin1":            "pt",

		// Names kbd and xkeyboard-config simply disagree on.
		"uk":        "gb",
		"sg":        "ch",
		"sf":        "ch",
		"la-latin1": "latam",
		"br-abnt2":  "br",
		"jp106":     "jp",
		"trq":       "tr",
		"cf":        "ca",
		"gr":        "gr",
		"nb":        "no",

		// Alternative US layouts are still the US layout as far as XKB's
		// kb_layout is concerned.
		"dvorak":  "us",
		"colemak": "us",

		// Apple variants.
		"mac-us":        "us",
		"mac-de-latin1": "de",

		// Anything unrecognisable falls back to a keyboard that works.
		"":              "us",
		"defkeymap":     "us",
		"something-odd": "us",
	}

	for keymap, want := range cases {
		if got := XKBLayout(keymap); got != want {
			t.Errorf("XKBLayout(%q) = %q, want %q", keymap, got, want)
		}
	}
}
