// Package episode reads the show and episode number from the name of a
// video file, as fansub groups and release names write them:
// "[Group] Sousou no Frieren - 05 (1080p) [ABCD1234].mkv",
// "Frieren.S01E05.1080p.WEB.x264-GRP.mkv", "Frieren Episode 5.mp4"; and
// from the title and address of a web page that plays one.
package episode

import (
	"net/url"
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
	// Web pages: only a spelled-out episode counts, in the title
	// ("Episode 5", "Ep. 5", "E5") or in the address ("show-episode-5").
	pageEpisode = regexp.MustCompile(`(?i)\b(?:(?:Episode|Ep\.?)\s?|E)(\d{1,4})\b`)
	pathEpisode = regexp.MustCompile(`(?i)(?:^|[-_])(?:episode|ep)[-_]?(\d{1,4})(?:$|[-_.])`)
	pageID      = regexp.MustCompile(`[-_]\d{5,}$`)
	watchWord   = regexp.MustCompile(`(?i)^\s*watch\s`)
)

// Parts of an address that name no show.
var pathWords = map[string]bool{"watch": true, "anime": true, "play": true, "video": true, "videos": true,
	"episode": true, "episodes": true, "stream": true, "series": true, "show": true, "shows": true,
	"embed": true, "sub": true, "dub": true, "player": true}

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
	n.setTitle(title)
	if n.Title == "" || n.Episode <= 0 {
		return Name{}, false
	}
	return n, true
}

// setTitle sets the title, and the season if the title ends in one.
func (n *Name) setTitle(title string) {
	title = strings.TrimRight(title, " -")
	if m := seasonWord.FindStringSubmatch(title); m != nil {
		n.Season, _ = strconv.Atoi(m[1] + m[2] + m[3])
		title = strings.TrimRight(title[:len(title)-len(m[0])], " -")
	}
	n.Title = strings.TrimSpace(strings.TrimLeft(title, " -"))
}

// ParsePage reads the title and the address of a web page that plays a
// video. Sites differ in where they say it: "Watch Sousou no Frieren Episode
// 5 English Sub - Site" as the title, or a title that is only the episode's
// own, with the rest in the address: ".../watch/sousou-no-frieren?ep=5",
// ".../sousou-no-frieren-episode-5". ok is false if neither gives an episode
// number and a show.
func ParsePage(title, address string) (n Name, ok bool) {
	s := " " + strings.Join(strings.Fields(groups.ReplaceAllString(title, " ")), " ")
	var show string
	if m := seasonEp.FindStringSubmatchIndex(s); m != nil {
		show = s[:m[0]]
		n.Season, _ = strconv.Atoi(s[m[2]:m[3]])
		n.Episode, _ = strconv.Atoi(s[m[4]:m[5]])
	} else if m := pageEpisode.FindStringSubmatchIndex(s); m != nil {
		show = s[:m[0]]
		n.Episode, _ = strconv.Atoi(s[m[2]:m[3]])
	}
	season := n.Season
	n.setTitle(strings.TrimRight(watchWord.ReplaceAllString(show, " "), " -–|:,·"))

	if u, err := url.Parse(address); err == nil && (n.Title == "" || n.Episode <= 0) {
		slug, number := pageSlug(u)
		if n.Episode <= 0 {
			n.Episode = number
		}
		if n.Title == "" {
			n.Season = season
			n.setTitle(" " + slug)
		}
	}
	if n.Title == "" || n.Episode <= 0 {
		return Name{}, false
	}
	return n, true
}

// pageSlug is the show an address names, as words, and the episode; "" and
// 0 for what it does not say. Sites number episodes with "?ep=5" or in the
// path; a longer number is an id of theirs.
func pageSlug(u *url.URL) (show string, number int) {
	for _, key := range []string{"ep", "episode"} {
		if v := u.Query().Get(key); len(v) <= 4 && number == 0 {
			number, _ = strconv.Atoi(v)
		}
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if m := pathEpisode.FindStringSubmatchIndex(part); m != nil {
			if number == 0 {
				number, _ = strconv.Atoi(part[m[2]:m[3]])
			}
			part = part[:m[0]]
		}
		part = pageID.ReplaceAllString(strings.TrimSuffix(part, path.Ext(part)), "")
		// A show is written in small letters with dashes; ids and the
		// site's own words are not.
		if show != "" || len(part) < 3 || part != strings.ToLower(part) || pathWords[part] ||
			!strings.ContainsFunc(part, unicode.IsLetter) {
			continue
		}
		show = strings.Join(strings.FieldsFunc(part, func(r rune) bool { return r == '-' || r == '_' || r == '+' }), " ")
	}
	return show, max(number, 0)
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
