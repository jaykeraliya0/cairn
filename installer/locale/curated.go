package locale

import (
	"sort"
	"strings"
)

// Layout is one entry in the keyboard picker: what the user reads, the console
// keymap it selects, and the Hyprland kb_layout that goes with it.
type Layout struct {
	// Label is what the picker shows, e.g. "German (Switzerland)".
	Label string
	// Keymap is the console keymap name for /etc/vconsole.conf.
	Keymap string
	// XKB is the Hyprland kb_layout. Curated entries carry it explicitly
	// rather than deriving it: the derivation is a heuristic, and these are
	// exactly the cases worth being certain about.
	XKB string
}

// commonLayouts is the readable front of the keyboard list.
//
// The raw kbd catalogue is 241 names like `amiga-de` and `applkey` in
// alphabetical order, which puts nothing useful on the first screen and asks
// the user to already know the code they want. English variants lead, then
// everything else A-Z.
//
// Every Keymap here is checked against the running system in Layouts(), so an
// entry kbd drops upstream disappears from the picker rather than failing at
// `loadkeys` time on the installed machine.
var commonLayouts = []Layout{
	{"English (US)", "us", "us"},
	{"English (UK)", "uk", "gb"},
	{"English (US, Dvorak)", "dvorak", "us"},
	{"English (US, Colemak)", "colemak", "us"},

	{"Belarusian", "by", "by"},
	{"Belgian", "be-latin1", "be"},
	{"Brazilian", "br-abnt2", "br"},
	{"Bulgarian", "bg_bds-utf8", "bg"},
	{"Croatian", "croat", "hr"},
	{"Czech", "cz", "cz"},
	{"Danish", "dk-latin1", "dk"},
	{"Dutch", "nl", "nl"},
	{"Estonian", "et", "ee"},
	{"Finnish", "fi", "fi"},
	{"French", "fr-latin1", "fr"},
	{"French (Canada)", "cf", "ca"},
	{"French (Switzerland)", "fr_CH", "ch"},
	{"Georgian", "ge", "ge"},
	{"German", "de-latin1", "de"},
	{"German (no dead keys)", "de-latin1-nodeadkeys", "de"},
	{"German (Switzerland)", "de_CH-latin1", "ch"},
	// kbd's "gr" is Greek, not German: its charset is iso-8859-7.
	{"Greek", "gr", "gr"},
	{"Hebrew", "il", "il"},
	{"Hungarian", "hu", "hu"},
	{"Icelandic", "is-latin1", "is"},
	{"Italian", "it", "it"},
	{"Japanese", "jp106", "jp"},
	{"Latin American", "la-latin1", "latam"},
	{"Latvian", "lv", "lv"},
	{"Lithuanian", "lt", "lt"},
	{"Macedonian", "mk", "mk"},
	{"Norwegian", "no", "no"},
	{"Polish", "pl", "pl"},
	{"Portuguese", "pt-latin1", "pt"},
	{"Romanian", "ro", "ro"},
	{"Russian", "ru", "ru"},
	{"Slovak", "sk-qwerty", "sk"},
	{"Slovenian", "slovene", "si"},
	{"Spanish", "es", "es"},
	{"Swedish", "sv-latin1", "se"},
	{"Turkish", "trq", "tr"},
	{"Ukrainian", "ua", "ua"},
}

// Layouts returns the keyboard picker's entries: the curated list first, then
// every other keymap the system has, alphabetically.
//
// Keeping the long tail means no keyboard becomes unreachable — the picker
// filters as you type — while the first screenful stays readable. Tail entries
// take their kb_layout from the XKBLayout heuristic, since there is nothing
// better to go on for a name nobody curated.
func Layouts(systemKeymaps []string) []Layout {
	available := make(map[string]bool, len(systemKeymaps))
	for _, keymap := range systemKeymaps {
		available[keymap] = true
	}

	curated := make(map[string]bool, len(commonLayouts))
	layouts := make([]Layout, 0, len(commonLayouts)+len(systemKeymaps))
	for _, layout := range commonLayouts {
		// A system without the keymap cannot load it, so do not offer it.
		if len(systemKeymaps) > 0 && !available[layout.Keymap] {
			continue
		}
		curated[layout.Keymap] = true
		layouts = append(layouts, layout)
	}

	var rest []Layout
	for _, keymap := range systemKeymaps {
		if curated[keymap] {
			continue
		}
		rest = append(rest, Layout{Label: keymap, Keymap: keymap, XKB: XKBLayout(keymap)})
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].Label < rest[j].Label })

	return append(layouts, rest...)
}

// commonLocaleLabels names the locales most people want, so the picker opens
// on something readable rather than `aa_DJ.UTF-8`. Anything not named here
// still appears, further down, under its own name.
var commonLocaleLabels = map[string]string{
	"en_US.UTF-8": "English (United States)",
	"en_GB.UTF-8": "English (United Kingdom)",
	"en_AU.UTF-8": "English (Australia)",
	"en_CA.UTF-8": "English (Canada)",
	"en_IE.UTF-8": "English (Ireland)",
	"en_IN.UTF-8": "English (India)",
	"en_NZ.UTF-8": "English (New Zealand)",
	"en_ZA.UTF-8": "English (South Africa)",

	"ar_EG.UTF-8": "Arabic (Egypt)",
	"bg_BG.UTF-8": "Bulgarian",
	"cs_CZ.UTF-8": "Czech",
	"da_DK.UTF-8": "Danish",
	"de_AT.UTF-8": "German (Austria)",
	"de_CH.UTF-8": "German (Switzerland)",
	"de_DE.UTF-8": "German (Germany)",
	"el_GR.UTF-8": "Greek",
	"es_ES.UTF-8": "Spanish (Spain)",
	"es_MX.UTF-8": "Spanish (Mexico)",
	"et_EE.UTF-8": "Estonian",
	"fi_FI.UTF-8": "Finnish",
	"fr_CA.UTF-8": "French (Canada)",
	"fr_CH.UTF-8": "French (Switzerland)",
	"fr_FR.UTF-8": "French (France)",
	"he_IL.UTF-8": "Hebrew",
	"hi_IN.UTF-8": "Hindi",
	"hr_HR.UTF-8": "Croatian",
	"hu_HU.UTF-8": "Hungarian",
	"id_ID.UTF-8": "Indonesian",
	"it_IT.UTF-8": "Italian",
	"ja_JP.UTF-8": "Japanese",
	"ko_KR.UTF-8": "Korean",
	"lt_LT.UTF-8": "Lithuanian",
	"lv_LV.UTF-8": "Latvian",
	"nb_NO.UTF-8": "Norwegian (Bokmal)",
	"nl_NL.UTF-8": "Dutch",
	"pl_PL.UTF-8": "Polish",
	"pt_BR.UTF-8": "Portuguese (Brazil)",
	"pt_PT.UTF-8": "Portuguese (Portugal)",
	"ro_RO.UTF-8": "Romanian",
	"ru_RU.UTF-8": "Russian",
	"sk_SK.UTF-8": "Slovak",
	"sl_SI.UTF-8": "Slovenian",
	"sr_RS.UTF-8": "Serbian",
	"sv_SE.UTF-8": "Swedish",
	"th_TH.UTF-8": "Thai",
	"tr_TR.UTF-8": "Turkish",
	"uk_UA.UTF-8": "Ukrainian",
	"vi_VN.UTF-8": "Vietnamese",
	"zh_CN.UTF-8": "Chinese (Simplified)",
	"zh_TW.UTF-8": "Chinese (Traditional)",
}

// Ordered fills in each locale's Label and returns them with the common ones
// first — English leading — and everything else after, under its own name.
func Ordered(locales []Locale) []Locale {
	var english, common, rest []Locale

	for _, l := range locales {
		label, isCommon := commonLocaleLabels[l.Name]
		entry := Locale{Name: l.Name, GenLine: l.GenLine, Label: label}
		switch {
		case !isCommon:
			entry.Label = l.Name
			rest = append(rest, entry)
		case strings.HasPrefix(l.Name, "en_"):
			english = append(english, entry)
		default:
			common = append(common, entry)
		}
	}

	byLabel := func(s []Locale) { sort.Slice(s, func(i, j int) bool { return s[i].Label < s[j].Label }) }
	byLabel(english)
	byLabel(common)
	byLabel(rest)

	out := make([]Locale, 0, len(locales))
	out = append(out, english...)
	out = append(out, common...)
	return append(out, rest...)
}
