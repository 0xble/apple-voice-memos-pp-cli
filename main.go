package main

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/spf13/cobra"
)

const (
	cliName           = "apple-voice-memos-pp-cli"
	version           = "0.1.0-local"
	coreDataEpochUnix = 978307200 // 2001-01-01T00:00:00Z
)

type config struct {
	DBPath        string
	RecordingsDir string
	JSON          bool
	Agent         bool
	NoColor       bool
}

type memo struct {
	ID            int64     `json:"id"`
	UUID          string    `json:"uuid,omitempty"`
	Title         string    `json:"title"`
	Date          time.Time `json:"date"`
	DurationSec   float64   `json:"duration_seconds"`
	Duration      string    `json:"duration"`
	Filename      string    `json:"filename"`
	Path          string    `json:"path"`
	HasTranscript bool      `json:"has_transcript"`
	Exists        bool      `json:"exists"`
}

var cfg config

func main() {
	root := &cobra.Command{
		Use:     cliName,
		Short:   "Local, read-only CLI for Apple Voice Memos on macOS",
		Version: version,
		Long: `Local, read-only CLI for Apple Voice Memos on macOS.

Reads the Voice Memos SQLite store and .m4a files synced by iCloud. By default it
uses Apple's modern path:
~/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings/CloudRecordings.db

No network calls. No writes to Apple's database. Export copies audio files only when requested.`,
	}
	root.PersistentFlags().StringVar(&cfg.DBPath, "db", "", "path to CloudRecordings.db")
	root.PersistentFlags().StringVar(&cfg.RecordingsDir, "recordings-dir", "", "directory containing Voice Memos .m4a files")
	root.PersistentFlags().BoolVar(&cfg.JSON, "json", false, "emit JSON")
	root.PersistentFlags().BoolVar(&cfg.Agent, "agent", false, "agent mode: JSON, no color, non-interactive")
	root.PersistentFlags().BoolVar(&cfg.NoColor, "no-color", false, "disable color output")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cfg.Agent {
			cfg.JSON = true
			cfg.NoColor = true
		}
		return nil
	}

	root.AddCommand(cmdDoctor(), cmdList(), cmdRecent(), cmdTranscript(), cmdExport(), cmdAgentContext(), cmdWhich())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func cmdDoctor() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check access to the local Apple Voice Memos store",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, recDir := resolvePaths()
			out := map[string]any{"cli": cliName, "version": version, "db_path": dbPath, "recordings_dir": recDir}
			if _, err := os.Stat(dbPath); err != nil {
				out["ok"] = false
				out["error"] = err.Error()
				printJSON(out)
				return fmt.Errorf("Voice Memos DB not accessible: %w", err)
			}
			db, err := openDB(dbPath)
			if err != nil {
				out["ok"] = false
				out["error"] = err.Error()
				printJSON(out)
				return err
			}
			defer db.Close()
			var count int
			if err := db.QueryRow("SELECT count(*) FROM ZCLOUDRECORDING").Scan(&count); err != nil {
				out["ok"] = false
				out["error"] = err.Error()
				printJSON(out)
				return err
			}
			out["ok"] = true
			out["recording_count"] = count
			printJSON(out)
			return nil
		},
	}
}

func cmdList() *cobra.Command {
	var limit, offset int
	var search, after, before, format string
	c := &cobra.Command{
		Use:   "list",
		Short: "List Voice Memos metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			memos, err := queryMemos(limit, offset, search, after, before)
			if err != nil {
				return err
			}
			switch format {
			case "json":
				printJSON(memos)
			case "csv":
				return printCSV(memos)
			default:
				printTable(memos)
			}
			return nil
		},
	}
	c.Flags().IntVar(&limit, "limit", 20, "maximum rows")
	c.Flags().IntVar(&offset, "offset", 0, "skip rows")
	c.Flags().StringVar(&search, "search", "", "case-insensitive title substring")
	c.Flags().StringVar(&after, "after", "", "only recordings on/after YYYY-MM-DD")
	c.Flags().StringVar(&before, "before", "", "only recordings on/before YYYY-MM-DD")
	c.Flags().StringVar(&format, "format", "table", "output format: table, json, csv")
	c.PreRun = func(cmd *cobra.Command, args []string) {
		if cfg.JSON || cfg.Agent {
			format = "json"
		}
	}
	return c
}

func cmdRecent() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "recent",
		Short: "List recent Voice Memos",
		RunE: func(cmd *cobra.Command, args []string) error {
			memos, err := queryMemos(limit, 0, "", "", "")
			if err != nil {
				return err
			}
			if cfg.JSON {
				printJSON(memos)
			} else {
				printTable(memos)
			}
			return nil
		},
	}
	c.Flags().IntVar(&limit, "limit", 10, "maximum rows")
	return c
}

func cmdTranscript() *cobra.Command {
	var raw bool
	c := &cobra.Command{
		Use:   "transcript <id|uuid|filename>",
		Short: "Extract Apple's embedded transcript from a memo .m4a tsrp atom",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := findMemo(args[0])
			if err != nil {
				return err
			}
			text, segments, err := extractTranscript(m.Path)
			if err != nil {
				return err
			}
			if cfg.JSON {
				printJSON(map[string]any{"memo": m, "transcript": text, "segments": segments})
				return nil
			}
			if raw {
				fmt.Print(text)
				if !strings.HasSuffix(text, "\n") {
					fmt.Println()
				}
				return nil
			}
			fmt.Printf("# %s\n\nDate: %s\nDuration: %s\nSource: %s\n\n%s\n", m.Title, m.Date.Format(time.RFC3339), m.Duration, m.Filename, text)
			return nil
		},
	}
	c.Flags().BoolVar(&raw, "raw", false, "print transcript only")
	return c
}

func cmdExport() *cobra.Command {
	var outDir string
	var overwrite bool
	c := &cobra.Command{
		Use:   "export <id|uuid|filename>",
		Short: "Copy a memo audio file to a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := findMemo(args[0])
			if err != nil {
				return err
			}
			if outDir == "" {
				outDir = "."
			}
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return err
			}
			name := safeFilename(strings.TrimSpace(m.Title))
			if name == "" {
				name = strings.TrimSuffix(m.Filename, filepath.Ext(m.Filename))
			}
			dest := filepath.Join(outDir, fmt.Sprintf("%s - %s%s", m.Date.Format("2006-01-02 150405"), name, filepath.Ext(m.Filename)))
			if !overwrite {
				if _, err := os.Stat(dest); err == nil {
					return fmt.Errorf("destination exists: %s (pass --overwrite)", dest)
				}
			}
			if err := copyFile(m.Path, dest); err != nil {
				return err
			}
			if cfg.JSON {
				printJSON(map[string]any{"source": m.Path, "destination": dest})
			} else {
				fmt.Println(dest)
			}
			return nil
		},
	}
	c.Flags().StringVar(&outDir, "out", ".", "output directory")
	c.Flags().BoolVar(&overwrite, "overwrite", false, "overwrite existing destination")
	return c
}

func cmdAgentContext() *cobra.Command {
	var pretty bool
	c := &cobra.Command{Use: "agent-context", Short: "Describe CLI capabilities for agents", RunE: func(cmd *cobra.Command, args []string) error {
		ctx := map[string]any{
			"cli": cliName, "version": version, "side_effects": "read-only except export copies selected audio files", "auth": "none", "network": false,
			"commands":   []string{"doctor", "list", "recent", "transcript", "export", "which", "agent-context"},
			"default_db": defaultDBPath(), "default_recordings_dir": defaultRecordingsDir(),
		}
		b, _ := json.Marshal(ctx)
		if pretty {
			b, _ = json.MarshalIndent(ctx, "", "  ")
		}
		fmt.Println(string(b))
		return nil
	}}
	c.Flags().BoolVar(&pretty, "pretty", false, "pretty-print JSON")
	return c
}

func cmdWhich() *cobra.Command {
	return &cobra.Command{Use: "which <capability>", Short: "Map a task to the right command", Args: cobra.ExactArgs(1), Run: func(cmd *cobra.Command, args []string) {
		q := strings.ToLower(args[0])
		var ans string
		switch {
		case strings.Contains(q, "transcript") || strings.Contains(q, "text") || strings.Contains(q, "summar"):
			ans = "transcript <id|uuid|filename>"
		case strings.Contains(q, "export") || strings.Contains(q, "copy") || strings.Contains(q, "audio"):
			ans = "export <id|uuid|filename> --out <dir>"
		case strings.Contains(q, "recent"):
			ans = "recent --limit 10"
		default:
			ans = "list --search <term> --json"
		}
		if cfg.JSON {
			printJSON(map[string]string{"command": ans})
		} else {
			fmt.Println(ans)
		}
	}}
}

func defaultRecordingsDir() string {
	return filepath.Join(os.Getenv("HOME"), "Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings")
}
func defaultDBPath() string { return filepath.Join(defaultRecordingsDir(), "CloudRecordings.db") }
func resolvePaths() (string, string) {
	rec := cfg.RecordingsDir
	if rec == "" {
		rec = defaultRecordingsDir()
	}
	db := cfg.DBPath
	if db == "" {
		db = filepath.Join(rec, "CloudRecordings.db")
	}
	return db, rec
}
func openDB(path string) (*sql.DB, error) { return sql.Open("sqlite", "file:"+path+"?mode=ro") }

func queryMemos(limit, offset int, search, after, before string) ([]memo, error) {
	dbPath, recDir := resolvePaths()
	db, err := openDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	clauses := []string{}
	args := []any{}
	if search != "" {
		clauses = append(clauses, "(COALESCE(ZENCRYPTEDTITLE, ZCUSTOMLABEL, ZPATH) LIKE ? COLLATE NOCASE)")
		args = append(args, "%"+search+"%")
	}
	if after != "" {
		ts, err := dateToCoreData(after, false)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, "ZDATE >= ?")
		args = append(args, ts)
	}
	if before != "" {
		ts, err := dateToCoreData(before, true)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, "ZDATE <= ?")
		args = append(args, ts)
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	q := "SELECT Z_PK, COALESCE(ZUNIQUEID,''), COALESCE(ZENCRYPTEDTITLE, ZCUSTOMLABEL, ZPATH, ''), ZDATE, COALESCE(ZDURATION, ZLOCALDURATION, 0), COALESCE(ZPATH,'') FROM ZCLOUDRECORDING" + where + " ORDER BY ZDATE DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memo
	for rows.Next() {
		var id int64
		var uuid, title string
		var zdate, dur float64
		var filename string
		if err := rows.Scan(&id, &uuid, &title, &zdate, &dur, &filename); err != nil {
			return nil, err
		}
		path := filepath.Join(recDir, filename)
		exists := fileExists(path)
		out = append(out, memo{ID: id, UUID: uuid, Title: title, Date: coreDataToTime(zdate), DurationSec: dur, Duration: formatDuration(dur), Filename: filename, Path: path, HasTranscript: exists && hasTranscript(path), Exists: exists})
	}
	return out, rows.Err()
}

func findMemo(key string) (memo, error) {
	memos, err := queryMemos(10000, 0, "", "", "")
	if err != nil {
		return memo{}, err
	}
	keyLower := strings.ToLower(key)
	if id, err := strconv.ParseInt(key, 10, 64); err == nil {
		for _, m := range memos {
			if m.ID == id {
				return m, nil
			}
		}
	}
	for _, m := range memos {
		if strings.EqualFold(m.UUID, key) || strings.EqualFold(m.Filename, key) {
			return m, nil
		}
	}
	var matches []memo
	for _, m := range memos {
		if strings.Contains(strings.ToLower(m.Title), keyLower) {
			matches = append(matches, m)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return memo{}, fmt.Errorf("ambiguous memo %q: %d title matches; use id or filename", key, len(matches))
	}
	return memo{}, fmt.Errorf("memo not found: %s", key)
}

func coreDataToTime(seconds float64) time.Time {
	return time.Unix(coreDataEpochUnix+int64(seconds), int64((seconds-math.Floor(seconds))*1e9)).In(time.Local)
}
func dateToCoreData(s string, endOfDay bool) (float64, error) {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return 0, fmt.Errorf("expected YYYY-MM-DD: %w", err)
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Second)
	}
	return float64(t.Unix() - coreDataEpochUnix), nil
}
func formatDuration(sec float64) string {
	total := int(sec + 0.5)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func readAtomHeader(r io.ReadSeeker) (typ string, size int64, header int64, err error) {
	buf := make([]byte, 8)
	if _, err = io.ReadFull(r, buf); err != nil {
		return "", 0, 0, err
	}
	size = int64(uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3]))
	typ = string(buf[4:8])
	header = 8
	if size == 1 {
		ext := make([]byte, 8)
		if _, err = io.ReadFull(r, ext); err != nil {
			return
		}
		size = int64(0)
		for _, b := range ext {
			size = (size << 8) | int64(b)
		}
		header = 16
	}
	return
}
func findAtom(r io.ReadSeeker, end int64, target string) (atomEnd int64, dataStart int64, err error) {
	for {
		pos, _ := r.Seek(0, io.SeekCurrent)
		if pos >= end {
			return 0, 0, os.ErrNotExist
		}
		typ, size, header, err := readAtomHeader(r)
		if err != nil {
			return 0, 0, err
		}
		if size <= 0 {
			return 0, 0, os.ErrNotExist
		}
		atomEnd = pos + size
		dataStart = pos + header
		if typ == target {
			return atomEnd, dataStart, nil
		}
		if _, err := r.Seek(atomEnd, io.SeekStart); err != nil {
			return 0, 0, err
		}
	}
}
func hasTranscript(path string) bool { _, _, err := readTranscriptJSON(path); return err == nil }
func readTranscriptJSON(path string) (map[string]any, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	end, _ := f.Seek(0, io.SeekEnd)
	f.Seek(0, io.SeekStart)
	moovEnd, _, err := findAtom(f, end, "moov")
	if err != nil {
		return nil, nil, errors.New("moov atom not found")
	}
	trakEnd, _, err := findAtom(f, moovEnd, "trak")
	if err != nil {
		return nil, nil, errors.New("trak atom not found")
	}
	udtaEnd, _, err := findAtom(f, trakEnd, "udta")
	if err != nil {
		return nil, nil, errors.New("udta atom not found")
	}
	tsrpEnd, dataStart, err := findAtom(f, udtaEnd, "tsrp")
	if err != nil {
		return nil, nil, errors.New("tsrp transcript atom not found")
	}
	f.Seek(dataStart, io.SeekStart)
	data := make([]byte, tsrpEnd-dataStart)
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, nil, err
	}
	data = bytes.Trim(data, "\x00\r\n\t ")
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, data, fmt.Errorf("parse tsrp JSON: %w", err)
	}
	return obj, data, nil
}

type segment struct {
	Time float64 `json:"time"`
	Text string  `json:"text"`
}

func extractTranscript(path string) (string, []segment, error) {
	obj, _, err := readTranscriptJSON(path)
	if err != nil {
		return "", nil, err
	}
	segments := extractSegments(obj["attributedString"])
	if len(segments) == 0 {
		return "", nil, errors.New("no transcript text segments found")
	}
	lines := groupSegments(segments)
	return strings.Join(lines, "\n"), segments, nil
}
func extractSegments(v any) []segment {
	var segs []segment
	switch x := v.(type) {
	case []any:
		cur := 0.0
		for i, item := range x {
			if s, ok := item.(string); ok {
				if i+1 < len(x) {
					if m, ok := x[i+1].(map[string]any); ok {
						if tr, ok := m["timeRange"].([]any); ok && len(tr) > 0 {
							cur = asFloat(tr[0])
						}
					}
				}
				if strings.TrimSpace(s) != "" {
					segs = append(segs, segment{cur, s})
				}
			}
		}
	case map[string]any:
		runs, _ := x["runs"].([]any)
		attrs, _ := x["attributeTable"].([]any)
		cur := 0.0
		for i := 0; i < len(runs); i += 2 {
			s, _ := runs[i].(string)
			if i+1 < len(runs) {
				idx := int(asFloat(runs[i+1]))
				if idx >= 0 && idx < len(attrs) {
					if m, ok := attrs[idx].(map[string]any); ok {
						if tr, ok := m["timeRange"].([]any); ok && len(tr) > 0 {
							cur = asFloat(tr[0])
						}
					}
				}
			}
			if strings.TrimSpace(s) != "" {
				segs = append(segs, segment{cur, s})
			}
		}
	}
	sort.SliceStable(segs, func(i, j int) bool { return segs[i].Time < segs[j].Time })
	return segs
}
func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}
func groupSegments(segs []segment) []string {
	var lines []string
	var parts []string
	lineTime := segs[0].Time
	prev := segs[0].Time
	flush := func() {
		txt := cleanText(strings.Join(parts, " "))
		if txt != "" {
			lines = append(lines, fmt.Sprintf("%s %s", fmtTS(lineTime), txt))
		}
		parts = nil
	}
	for _, s := range segs {
		txt := strings.TrimSpace(s.Text)
		if txt == "" {
			continue
		}
		gap := s.Time - prev
		words := len(strings.Fields(strings.Join(parts, " ")))
		joined := strings.Join(parts, " ")
		if len(parts) > 0 && shouldBreak(joined, gap, words) {
			flush()
			lineTime = s.Time
		}
		parts = append(parts, txt)
		prev = s.Time
	}
	if len(parts) > 0 {
		flush()
	}
	return lines
}
func shouldBreak(text string, gap float64, words int) bool {
	t := strings.TrimSpace(text)
	if words >= 25 {
		return true
	}
	if words >= 4 && (strings.HasSuffix(t, ".") || strings.HasSuffix(t, "!") || strings.HasSuffix(t, "?")) {
		return true
	}
	if gap >= 4 && words >= 6 {
		return true
	}
	if gap >= 2 && words >= 12 {
		return true
	}
	return false
}
func fmtTS(sec float64) string {
	total := int(sec)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("[%d:%02d:%02d]", h, m, s)
	}
	return fmt.Sprintf("[%d:%02d]", m, s)
}

var multiSpace = regexp.MustCompile(`\s+`)
var filler = regexp.MustCompile(`(?i)\b(uh|um)\b\.?\s*`)

func cleanText(s string) string {
	s = strings.ReplaceAll(s, "...", " ")
	s = filler.ReplaceAllString(s, "")
	s = multiSpace.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, " ,", ",")
	s = strings.ReplaceAll(s, " .", ".")
	return strings.TrimSpace(s)
}

func printJSON(v any) { b, _ := json.MarshalIndent(v, "", "  "); fmt.Println(string(b)) }
func printTable(memos []memo) {
	fmt.Printf("%-5s  %-20s  %-8s  %-3s  %s\n", "ID", "DATE", "DURATION", "TXT", "TITLE")
	for _, m := range memos {
		txt := "no"
		if m.HasTranscript {
			txt = "yes"
		}
		fmt.Printf("%-5d  %-20s  %-8s  %-3s  %s\n", m.ID, m.Date.Format("2006-01-02 15:04"), m.Duration, txt, m.Title)
	}
}
func printCSV(memos []memo) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	w.Write([]string{"id", "uuid", "title", "date", "duration", "filename", "path", "has_transcript", "exists"})
	for _, m := range memos {
		w.Write([]string{strconv.FormatInt(m.ID, 10), m.UUID, m.Title, m.Date.Format(time.RFC3339), m.Duration, m.Filename, m.Path, strconv.FormatBool(m.HasTranscript), strconv.FormatBool(m.Exists)})
	}
	return w.Error()
}
func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

var unsafeFile = regexp.MustCompile(`[^a-zA-Z0-9._ -]+`)

func safeFilename(s string) string {
	s = unsafeFile.ReplaceAllString(s, "_")
	s = strings.Trim(s, " .")
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}
