package episode

import "testing"

func TestParse(t *testing.T) {
	for name, want := range map[string]Name{
		"[SubsPlease] Sousou no Frieren - 05 (1080p) [ABCD1234].mkv":             {"Sousou no Frieren", 0, 5},
		"[Erai-raws] Dandadan - 07 [1080p][Multiple Subtitle][ABCDEF01].mkv":     {"Dandadan", 0, 7},
		"Frieren.S01E05.1080p.WEB.x264-GRP.mkv":                                  {"Frieren", 1, 5},
		"Spy x Family S02E03 1080p.mkv":                                          {"Spy x Family", 2, 3},
		"[Judas] Vinland Saga S2 - 12v2.mkv":                                     {"Vinland Saga", 2, 12},
		"Kusuriya no Hitorigoto 2nd Season - 03.mkv":                             {"Kusuriya no Hitorigoto", 2, 3},
		"One Piece - 1100 [1080p].mkv":                                           {"One Piece", 0, 1100},
		"Frieren Episode 5.mp4":                                                  {"Frieren", 0, 5},
		"Frieren - Ep05.mkv":                                                     {"Frieren", 0, 5},
		"Frieren_-_05_[720p].mkv":                                                {"Frieren", 0, 5},
		"[Group] Mushoku Tensei II - Isekai Ittara Honki Dasu - 13 [1080p].mkv":  {"Mushoku Tensei II - Isekai Ittara Honki Dasu", 0, 13},
		"86 - Eighty Six - 05.mkv":                                               {"86 - Eighty Six", 0, 5},
		"[Group] Show Name 05 [1080p].mkv":                                       {"Show Name", 0, 5},
		"Show Name (2024) - 05.mkv":                                              {"Show Name", 0, 5},
		"Show Name 05 1080p WEB-DL AAC.mkv":                                      {"Show Name", 0, 5},
		"/home/neko/Downloads/[ASW] Kaiju No. 8 - 02 [1080p HEVC][1234ABCD].mkv": {"Kaiju No. 8", 0, 2},
	} {
		got, ok := Parse(name)
		if !ok || got != want {
			t.Errorf("%q: got %+v %v, want %+v", name, got, ok, want)
		}
	}
	for _, name := range []string{"Your Name (2016).mkv", "Weathering With You 2019.mkv", "holiday.mp4", "[Group] - 05.mkv", "Show - 00.mkv"} {
		if got, ok := Parse(name); ok {
			t.Errorf("%q: got %+v", name, got)
		}
	}
}

func TestParsePage(t *testing.T) {
	for _, c := range []struct {
		title, address string
		want           Name
	}{
		{"The Magic That Started Everything", "https://www.miruro.cx/watch/GSmujr7tGD1U1muXEcm5kH9_QSSiAUOI/witch-hat-atelier?ep=1", Name{"witch hat atelier", 0, 1}},
		{"Watch Sousou no Frieren Episode 5 English Subbed at Site", "https://site.example/sousou-no-frieren-episode-5", Name{"Sousou no Frieren", 0, 5}},
		{"Frieren: Beyond Journey's End Season 2 Episode 3 - A Title - Site", "https://site.example/watch/G9DUE0QW7", Name{"Frieren: Beyond Journey's End", 2, 3}},
		{"Spy x Family (Dub) E12 - Penguin Park | Site", "", Name{"Spy x Family", 0, 12}},
		{"Episode 7", "https://site.example/anime/dandadan-season-2/ep-7", Name{"dandadan", 2, 7}},
		{"Some title", "https://site.example/watch/kaiju-no-8-19145/episode-4.html", Name{"kaiju no 8", 0, 4}},
		{"Vinland Saga S02E12", "https://site.example/v/abc", Name{"Vinland Saga", 2, 12}},
	} {
		got, ok := ParsePage(c.title, c.address)
		if !ok || got != c.want {
			t.Errorf("%q %q: got %+v %v, want %+v", c.title, c.address, got, ok, c.want)
		}
	}
	for _, c := range [][2]string{
		{"Watch Frieren English Sub/Dub online Free on Site", "https://site.example/watch/frieren-18542?ep=107257"},
		{"Top 10 Anime of 2024 - YouTube", "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		{"Episode 5", "https://site.example/watch/AbC123"},
		{"Lofi beats - 24 hours", ""},
	} {
		if got, ok := ParsePage(c[0], c[1]); ok {
			t.Errorf("%q %q: got %+v", c[0], c[1], got)
		}
	}
}

func TestAliasAndQuery(t *testing.T) {
	n := Name{Title: "Kaiju No. 8", Season: 2, Episode: 1}
	if n.Alias() != "kaiju no 8 s2" || n.Query() != "Kaiju No. 8 2" {
		t.Fatalf("%q %q", n.Alias(), n.Query())
	}
	if n := (Name{Title: "Sousou no Frieren", Season: 1}); n.Alias() != "sousou no frieren" || n.Query() != "Sousou no Frieren" {
		t.Fatalf("%q %q", n.Alias(), n.Query())
	}
}

func TestTemplate(t *testing.T) {
	for _, c := range []struct {
		address string
		n       int
		want    string
	}{
		{"https://site.example/watch/AbC_123/sousou-no-frieren?ep=5", 5, "https://site.example/watch/AbC_123/sousou-no-frieren?ep={ep}"},
		{"https://site.example/w?id=abc&episode=12&lang=sub#top", 12, "https://site.example/w?id=abc&episode={ep}&lang=sub"},
		{"https://site.example/sousou-no-frieren-episode-5", 5, "https://site.example/sousou-no-frieren-episode-{ep}"},
		{"https://site.example/watch/kaiju-no-8-19145/episode-4.html?s=1", 4, "https://site.example/watch/kaiju-no-8-19145/episode-{ep}.html?s=1"},
		{"https://site.example/anime/dandadan-season-2/ep-7", 7, "https://site.example/anime/dandadan-season-2/ep-{ep}"},
		// A site that counts differently: its episode 10 is the show's 9th.
		{"https://site.example/watch/sRD4L/steel-ball-run?ep=10", 9, "https://site.example/watch/sRD4L/steel-ball-run?ep={ep+1}"},
		{"https://site.example/frieren-episode-5", 6, "https://site.example/frieren-episode-{ep-1}"},
		// Not said: nothing to go on.
		{"https://site.example/watch/frieren-18542?ep=107257", 5, ""},
		{"https://site.example/watch/AbC123", 5, ""},
		{"file:///home/x/frieren-episode-5.mkv", 5, ""},
	} {
		if got := Template(c.address, c.n); got != c.want {
			t.Errorf("%q %d: got %q, want %q", c.address, c.n, got, c.want)
		}
	}
	if got := TemplateOf("https://site.example/watch?id=abc&ep=12"); got != "https://site.example/watch?id=abc&ep={ep}" {
		t.Errorf("of: %q", got)
	}
	if got := Fill("https://site.example/frieren-episode-{ep}", 6); got != "https://site.example/frieren-episode-6" {
		t.Errorf("fill: %q", got)
	}
	if got := Fill("https://site.example/w?ep={ep+1}", 10); got != "https://site.example/w?ep=11" {
		t.Errorf("fill ahead: %q", got)
	}
	if got := Fill("https://site.example/frieren-episode-{ep-12}", 14); got != "https://site.example/frieren-episode-2" {
		t.Errorf("fill behind: %q", got)
	}
	if !IsTemplate("https://site.example/w?ep={ep+1}") || IsTemplate("https://site.example/w?ep=1") {
		t.Error("is template")
	}
}
