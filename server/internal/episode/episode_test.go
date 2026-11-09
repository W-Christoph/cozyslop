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

func TestAliasAndQuery(t *testing.T) {
	n := Name{Title: "Kaiju No. 8", Season: 2, Episode: 1}
	if n.Alias() != "kaiju no 8 s2" || n.Query() != "Kaiju No. 8 2" {
		t.Fatalf("%q %q", n.Alias(), n.Query())
	}
	if n := (Name{Title: "Sousou no Frieren", Season: 1}); n.Alias() != "sousou no frieren" || n.Query() != "Sousou no Frieren" {
		t.Fatalf("%q %q", n.Alias(), n.Query())
	}
}
