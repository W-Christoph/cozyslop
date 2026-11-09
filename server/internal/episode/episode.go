// Package episode reads the show and episode number from the name of a
// video file, as fansub groups and release names write them:
// "[Group] Sousou no Frieren - 05 (1080p) [ABCD1234].mkv",
// "Frieren.S01E05.1080p.WEB.x264-GRP.mkv", "Frieren Episode 5.mp4".
package episode

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Name is what a file name says.
type Name struct {
	Title   string // as written, e.g. "Sousou no Frieren"
	Season  int    // 0 if not given
	Episode int
}

var (
	groups      = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)|\{[^}]*\}|【[^】]*】`)
	seasonEp    = regexp.MustCompile(`(?i)\bS(\d{1,2})\s?E(\d{1,4})(?:v\d)?\b`)
	episodeWord = regexp.MustCompile(`(?i)\b(?:Episode|Ep\.?|E)\s?(\d{1,4})(?:v\d)?\b`)
	dashNumber  = regexp.MustCompile(`\s-\s(\d{1,4})(?:v\d)?(?:\s|$)`)
	lastNumber  = regexp.MustCompile(`\s(\d{1,4})(?:v\d)?$`)
	technical   = regexp.MustCompile(`(?i)\s(?:\d{3,4}p|4k|uhd|x26[45]|h\.?26[45]|hevc|avc|av1|web(?:-?dl|-?rip)?|blu-?ray|bd(?:rip)?|hdtv|aac|flac|opus|ddp?5\.1|\d+bit|hdr|dual(?:-audio)?|multi(?:-subs?)?|uncensored|-\S+)$`)
	seasonWord  = regexp.MustCompile(`(?i)\s(?:S(\d{1,2})|Season\s(\d{1,2})|(\d{1,2})(?:st|nd|rd|th)\sSeason)$`)
)

// Parse reads name, a file name with or without its folder. ok is false if
// it gives no episode number.
func Parse(name string) (n Name, ok bool) {
	s := path.Base(strings.ReplaceAll(name, `\`, "/"))
	if ext := path.Ext(s); len(ext) >= 3 && len(ext) <= 5 {
		s = strings.TrimSuffix(s, ext)
	}
	s = strings.ReplaceAll(s, "_", " ")
	if !strings.Contains(s, " ") {
		s = strings.ReplaceAll(s, ".", " ") // release names: Show.S01E05.1080p
	}
	s = " " + strings.Join(strings.Fields(groups.ReplaceAllString(s, " ")), " ")

	var title, number string
	if m := seasonEp.FindStringSubmatchIndex(s); m != nil {
		title, number = s[:m[0]], s[m[4]:m[5]]
		n.Season, _ = strconv.Atoi(s[m[2]:m[3]])
	} else if m := episodeWord.FindStringSubmatchIndex(s); m != nil {
		title, number = s[:m[0]], s[m[2]:m[3]]
	} else if all := dashNumber.FindAllStringSubmatchIndex(s, -1); all != nil {
		m := all[len(all)-1]
		title, number = s[:m[0]], s[m[2]:m[3]]
	} else {
		for t := technical.FindStringIndex(s); t != nil; t = technical.FindStringIndex(s) {
			s = s[:t[0]]
		}
		m := lastNumber.FindStringSubmatchIndex(s)
		if m == nil {
			return Name{}, false
		}
		title, number = s[:m[0]], s[m[2]:m[3]]
		if len(number) == 4 && (number[:2] == "19" || number[:2] == "20") {
			return Name{}, false // a year
		}
	}
	n.Episode, _ = strconv.Atoi(number)
	title = strings.TrimRight(title, " -")
	if m := seasonWord.FindStringSubmatch(title); m != nil {
		n.Season, _ = strconv.Atoi(m[1] + m[2] + m[3])
		title = strings.TrimRight(title[:len(title)-len(m[0])], " -")
	}
	n.Title = strings.TrimSpace(strings.TrimLeft(title, " -"))
	if n.Title == "" || n.Episode <= 0 {
		return Name{}, false
	}
	return n, true
}

// Alias is the title as the watchlog remembers it: lowercase letters and
// digits, and the season if it is not the first.
func (n Name) Alias() string {
	words := strings.FieldsFunc(strings.ToLower(n.Title), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if n.Season > 1 {
		words = append(words, "s"+strconv.Itoa(n.Season))
	}
	return strings.Join(words, " ")
}

// Query is what to search AniList for.
func (n Name) Query() string {
	if n.Season > 1 {
		return n.Title + " " + strconv.Itoa(n.Season)
	}
	return n.Title
}
