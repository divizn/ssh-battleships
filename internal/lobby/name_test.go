package lobby

import "testing"

func TestCleanNameKeepsOnlyWhatIsSafeToPrint(t *testing.T) {
	cases := map[string]string{
		"Alice":                        "Alice",
		"  bob  ":                      "bob",
		"a-b_c.1":                      "a-b_c.1",
		"\x1b[31mred\x1b[0m":           "31mred0m",
		"line\r\nbreak":                "linebreak",
		"Zoë":                          "Zo",
		"":                             "",
		"\x07\x07":                     "",
		"averylongnamethatgoesonandon": "averylongnam",
	}
	for in, want := range cases {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := CleanName("averylongnamethatgoesonandon"); len(got) > NameLimit {
		t.Errorf("CleanName returned %d characters, want at most %d", len(got), NameLimit)
	}
}
