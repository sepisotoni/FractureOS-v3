package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
	"time"
	"unicode"
)

type mediaDeviceStatus struct {
	LyricsView bool   `json:"lyrics_view"`
	LyricsPage int    `json:"lyrics_page"`
	Enabled    *int   `json:"media_enabled"`
	View       bool   `json:"media_view"`
	Menu       int    `json:"menu"`
	Art        int    `json:"media_art_loaded"`
	Pending    bool   `json:"media_pending"`
	Sequence   uint   `json:"media_seq"`
	Action     string `json:"media_action"`
	Argument   int    `json:"media_arg"`
}
type mediaSnapshot struct {
	PositionMS *int64 `json:"position_ms,omitempty"`
	SampledAt  int64  `json:"sampled_at,omitempty"`
	Album      string `json:"album"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Message    string `json:"message"`
	Playing    bool   `json:"playing"`
	Position   int    `json:"position"`
	Duration   int    `json:"duration"`
	Volume     int    `json:"volume"`
	VolumeOK   bool   `json:"volume_ok"`
	PlayOK     bool   `json:"play_ok"`
	PreviousOK bool   `json:"previous_ok"`
	NextOK     bool   `json:"next_ok"`
	Artwork    []byte `json:"artwork,omitempty"`
	ClearArt   bool   `json:"clear_art"`
	Clear      bool   `json:"clear"`
}

func asciiHex(s string) string {
	var text [80]byte
	length := 0
	appendText := func(value string) {
		for i := 0; i < len(value) && length < len(text); i++ {
			text[length] = value[i]
			length++
		}
	}
	unsupported := false
	for _, r := range s {
		if length == len(text) {
			break
		}
		if replacement, ok := mediaASCIIFallback[r]; ok {
			appendText(replacement)
			unsupported = false
			continue
		}
		if r >= 32 && r <= 126 {
			text[length] = byte(r)
			length++
			unsupported = false
			continue
		}
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			if length > 0 && text[length-1] != ' ' {
				appendText(" ")
			}
			unsupported = false
			continue
		}
		if !unsupported && length > 0 && text[length-1] != ' ' {
			appendText(" ")
		}
		unsupported = true
	}
	start, end := 0, length
	for start < end && text[start] == ' ' {
		start++
	}
	for end > start && text[end-1] == ' ' {
		end--
	}
	if start == end && strings.TrimSpace(s) != "" {
		return hex.EncodeToString([]byte("[text unavailable]"))
	}
	if start == end {
		return "-"
	}
	return hex.EncodeToString(text[start:end])
}

var mediaASCIIFallback = map[rune]string{
	// Latin accented
	'À': "A", 'Á': "A", 'Â': "A", 'Ã': "A", 'Ä': "A", 'Å': "A", 'Ā': "A", 'Ă': "A", 'Ą': "A",
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'ā': "a", 'ă': "a", 'ą': "a",
	'Ç': "C", 'Ć': "C", 'Ĉ': "C", 'Č': "C", 'ç': "c", 'ć': "c", 'ĉ': "c", 'č': "c",
	'Ð': "D", 'Ď': "D", 'ð': "d", 'ď': "d",
	'È': "E", 'É': "E", 'Ê': "E", 'Ë': "E", 'Ē': "E", 'Ĕ': "E", 'Ė': "E", 'Ę': "E", 'Ě': "E",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ĕ': "e", 'ė': "e", 'ę': "e", 'ě': "e",
	'Ì': "I", 'Í': "I", 'Î': "I", 'Ï': "I", 'Ī': "I", 'Į': "I", 'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ī': "i", 'į': "i",
	'Ł': "L", 'Ľ': "L", 'ł': "l", 'ľ': "l",
	'Ñ': "N", 'Ń': "N", 'Ň': "N", 'ñ': "n", 'ń': "n", 'ň': "n",
	'Ò': "O", 'Ó': "O", 'Ô': "O", 'Õ': "O", 'Ö': "O", 'Ø': "O", 'Ō': "O", 'Ő': "O",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'ō': "o", 'ő': "o",
	'Ŕ': "R", 'Ř': "R", 'ŕ': "r", 'ř': "r",
	'Ś': "S", 'Ş': "S", 'Š': "S", 'ś': "s", 'ş': "s", 'š': "s",
	'ß': "ss", 'Ţ': "T", 'Ť': "T", 'ţ': "t", 'ť': "t",
	'Ù': "U", 'Ú': "U", 'Û': "U", 'Ü': "U", 'Ū': "U", 'Ů': "U", 'Ű': "U",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ū': "u", 'ů': "u", 'ű': "u",
	'Ý': "Y", 'Ÿ': "Y", 'ý': "y", 'ÿ': "y", 'Ž': "Z", 'Ź': "Z", 'Ż': "Z", 'ž': "z", 'ź': "z", 'ż': "z",
	'Æ': "AE", 'æ': "ae", 'Œ': "OE", 'œ': "oe", 'Þ': "Th", 'þ': "th",
	'Ğ': "G", 'ğ': "g", 'İ': "I", 'ı': "i", 'đ': "d", 'Đ': "D", 'ƒ': "f",
	// Cyrillic transliteration
	'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D", 'Е': "E", 'Ё': "Yo", 'Ж': "Zh",
	'З': "Z", 'И': "I", 'Й': "Y", 'К': "K", 'Л': "L", 'М': "M", 'Н': "N", 'О': "O",
	'П': "P", 'Р': "R", 'С': "S", 'Т': "T", 'У': "U", 'Ф': "F", 'Х': "Kh", 'Ц': "Ts",
	'Ч': "Ch", 'Ш': "Sh", 'Щ': "Shch", 'Ъ': "", 'Ы': "Y", 'Ь': "", 'Э': "E", 'Ю': "Yu", 'Я': "Ya",
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo", 'ж': "zh",
	'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o",
	'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts",
	'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
	'Ґ': "G", 'ґ': "g", 'Є': "Ye", 'є': "ye", 'І': "I", 'і': "i", 'Ї': "Yi", 'ї': "yi",
	// Greek transliteration
	'Α': "A", 'Β': "B", 'Γ': "G", 'Δ': "D", 'Ε': "E", 'Ζ': "Z", 'Η': "I", 'Θ': "Th",
	'Ι': "I", 'Κ': "K", 'Λ': "L", 'Μ': "M", 'Ν': "N", 'Ξ': "X", 'Ο': "O", 'Π': "P",
	'Ρ': "R", 'Σ': "S", 'Τ': "T", 'Υ': "Y", 'Φ': "F", 'Χ': "Ch", 'Ψ': "Ps", 'Ω': "O",
	'α': "a", 'β': "b", 'γ': "g", 'δ': "d", 'ε': "e", 'ζ': "z", 'η': "i", 'θ': "th",
	'ι': "i", 'κ': "k", 'λ': "l", 'μ': "m", 'ν': "n", 'ξ': "x", 'ο': "o", 'π': "p",
	'ρ': "r", 'σ': "s", 'ς': "s", 'τ': "t", 'υ': "y", 'φ': "f", 'χ': "ch", 'ψ': "ps", 'ω': "o",
	// Fullwidth ASCII and CJK punctuation (map to ASCII equivalents)
	'～': "~", '＆': "&", '！': "!", '？': "?", '、': ",", '。': ".",
	'（': "(", '）': ")", '：': ":", '；': ";",
	'０': "0", '１': "1", '２': "2", '３': "3", '４': "4",
	'５': "5", '６': "6", '７': "7", '８': "8", '９': "9",
	// Common symbols
	'©': "(c)", '®': "(R)", '™': "TM", '°': "deg",
	'×': "x", '÷': "/", '±': "+/-", '¡': "!", '¿': "?",
	'€': "EUR", '£': "GBP", '¥': "JPY", '¢': "c",
	'·': ".", '•': "*", '«': "<<", '»': ">>",
	'¹': "1", '²': "2", '³': "3", '¼': "1/4", '½': "1/2", '¾': "3/4",
	// Typography
	'\u2018': "'", '\u2019': "'", '\u201a': "'", '\u201c': "\"", '\u201d': "\"", '\u201e': "\"",
	'\u2010': "-", '\u2011': "-", '\u2012': "-", '\u2013': "-", '\u2014': "-", '\u2026': "...",
	'\u2039': "<", '\u203a': ">",
	'\u00a0': " ", '\u2002': " ", '\u2003': " ", '\u2009': " ",
}


func mediaTextValid(s string) bool {
	if s == "-" {
		return true
	}
	if len(s) > 160 || len(s)%2 != 0 {
		return false
	}
	b, e := hex.DecodeString(s)
	if e != nil {
		return false
	}
	for _, c := range b {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}
func mediaAPI(b *Broker, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var s mediaSnapshot
	if json.NewDecoder(r.Body).Decode(&s) != nil || s.Position < 0 || s.Duration < 0 || s.Duration > 86400 || s.Position > s.Duration || s.Volume < -1 || s.Volume > 100 || len(s.Title) > 512 || len(s.Artist) > 512 || len(s.Album) > 512 || len(s.Message) > 512 || (s.PositionMS != nil && (*s.PositionMS < 0 || *s.PositionMS > int64(s.Duration)*1000)) || (len(s.Artwork) != 0 && len(s.Artwork) != 18432) {
		http.Error(w, "invalid media update", 400)
		return
	}
	bit := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	received := time.Now()
	positionMS := int64(s.Position) * 1000
	if s.PositionMS != nil {
		positionMS = *s.PositionMS
	}
	// Timestamped Windows samples account for transport delay, bounded against
	// clock skew. Older companions retain their integer-second fallback.
	if s.Playing && s.SampledAt > 0 {
		age := received.UnixMilli() - s.SampledAt
		if age > 0 && age < 15000 {
			positionMS += age
		}
	}
	if positionMS > int64(s.Duration)*1000 {
		positionMS = int64(s.Duration) * 1000
	}
	commands := []string{}
	if s.Clear {
		commands = append(commands, "media-clear")
	} else {
		commands = append(commands, "media-text title "+asciiHex(s.Title), "media-text artist "+asciiHex(s.Artist), fmt.Sprintf("media-state %d,%d,%d,%d,%d,%d,%d,%d", bit(s.Playing), s.Position, s.Duration, s.Volume, bit(s.VolumeOK), bit(s.PlayOK), bit(s.PreviousOK), bit(s.NextOK)))
	}
	commands = append(commands, "media-text message "+asciiHex(s.Message))
	if !s.Clear && s.PositionMS != nil {
		commands = append(commands, fmt.Sprintf("media-clock %d,%d", positionMS+int64(bit(s.Playing))*time.Since(received).Milliseconds(), bit(s.Playing)))
	}
	if s.ClearArt {
		commands = append(commands, "media-art clear")
	}
	if e := b.batch(commands); e != nil {
		http.Error(w, "device update failed", 502)
		return
	}

	b.mediaMu.Lock()
	if s.Clear {
		b.mediaLatest = lyricsSession{}
		b.providedLyrics = nil
	} else {
		if b.mediaLatest.Title != s.Title || b.mediaLatest.Artist != s.Artist {
			b.providedLyrics = nil
		}
		b.mediaLatest = lyricsSession{Title: s.Title, Artist: s.Artist, Album: s.Album, Position: int(positionMS / 1000), PositionMS: positionMS, Duration: s.Duration, Playing: s.Playing}
		b.mediaReceived = received
	}
	b.mediaMu.Unlock()

	if len(s.Artwork) > 0 {
		if e := uploadMediaArt(b.device, s.Artwork); e != nil {
			http.Error(w, "artwork transfer failed", 502)
			return
		}
		if _, e := b.call("media-art load"); e != nil {
			http.Error(w, "artwork no longer requested", 409)
			return
		}
	}
	fmt.Fprint(w, `{"updated":true}`)
}
func thumbnailRGB(data []byte) ([]byte, error) { return thumbnailRGBSize(data, 96) }
func thumbnailRGBSize(data []byte, size int) ([]byte, error) {
	if len(data) > 2*1024*1024 {
		return nil, errors.New("thumbnail too large")
	}
	cfg, _, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 {
		return nil, errors.New("invalid thumbnail")
	}
	im, _, e := image.Decode(bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	out := make([]byte, size*size*2)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, b, _ := im.At(im.Bounds().Min.X+x*cfg.Width/size, im.Bounds().Min.Y+y*cfg.Height/size).RGBA()
			v := uint16((r>>11)<<11 | (g>>10)<<5 | (b >> 11))
			at := (y*size + x) * 2
			out[at] = byte(v)
			out[at+1] = byte(v >> 8)
		}
	}
	return out, nil
}
