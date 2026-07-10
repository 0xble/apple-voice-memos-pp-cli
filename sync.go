package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type storeSnapshot struct {
	Count    int       `json:"recording_count"`
	Latest   float64   `json:"-"`
	LatestAt time.Time `json:"latest_recording_at,omitempty"`
	DBMod    time.Time `json:"db_modified_at,omitempty"`
	WALMod   time.Time `json:"wal_modified_at,omitempty"`
}

type syncOptions struct {
	DaemonWait   time.Duration
	AppWait      time.Duration
	PollInterval time.Duration
}

type syncHooks struct {
	Snapshot        func() (storeSnapshot, error)
	KickDaemon      func() error
	AppRunning      func() (bool, error)
	LaunchHidden    func() error
	QuitLaunchedApp func() error
	Sleep           func(time.Duration)
}

type syncResult struct {
	Synced      bool          `json:"synced"`
	Changed     bool          `json:"changed"`
	Method      string        `json:"method"`
	AppLaunched bool          `json:"app_launched"`
	AppQuit     bool          `json:"app_quit"`
	Before      storeSnapshot `json:"before"`
	After       storeSnapshot `json:"after"`
	ElapsedMS   int64         `json:"elapsed_ms"`
	Warning     string        `json:"warning,omitempty"`
}

func syncStore(ctx context.Context, opts syncOptions, hooks syncHooks) (syncResult, error) {
	started := time.Now()
	before, err := hooks.Snapshot()
	if err != nil {
		return syncResult{}, err
	}
	result := syncResult{Before: before, After: before, Method: "voicememod"}
	if err := hooks.KickDaemon(); err != nil {
		result.Warning = "could not kick voicememod: " + err.Error()
	}
	if after, changed, err := pollForChange(ctx, before, opts.DaemonWait, opts.PollInterval, hooks); err != nil {
		return result, err
	} else if changed {
		result.Synced, result.Changed, result.After = true, true, after
		result.ElapsedMS = time.Since(started).Milliseconds()
		return result, nil
	}

	running, err := hooks.AppRunning()
	if err != nil {
		return result, err
	}
	if !running {
		if err := hooks.LaunchHidden(); err != nil {
			return result, fmt.Errorf("launch Voice Memos hidden: %w", err)
		}
		result.AppLaunched = true
		result.Method = "hidden-app"
	}

	after, changed, pollErr := pollForChange(ctx, before, opts.AppWait, opts.PollInterval, hooks)
	if result.AppLaunched {
		if err := hooks.QuitLaunchedApp(); err == nil {
			result.AppQuit = true
		} else if result.Warning == "" {
			result.Warning = "could not quit CLI-launched Voice Memos: " + err.Error()
		}
	}
	if pollErr != nil {
		return result, pollErr
	}
	result.After = after
	result.Changed = changed
	result.Synced = changed
	if !changed && result.Warning == "" {
		result.Warning = "store did not change during the sync window; it may already be current"
	}
	result.ElapsedMS = time.Since(started).Milliseconds()
	return result, nil
}

func pollForChange(ctx context.Context, before storeSnapshot, wait, interval time.Duration, hooks syncHooks) (storeSnapshot, bool, error) {
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	attempts := int(math.Ceil(float64(wait)/float64(interval))) + 1
	if attempts < 1 {
		attempts = 1
	}
	last := before
	for i := 0; i < attempts; i++ {
		select {
		case <-ctx.Done():
			return last, false, ctx.Err()
		default:
		}
		current, err := hooks.Snapshot()
		if err != nil {
			return last, false, err
		}
		last = current
		if snapshotsDiffer(before, current) {
			return current, true, nil
		}
		if i+1 < attempts {
			hooks.Sleep(interval)
		}
	}
	return last, false, nil
}

func snapshotsDiffer(a, b storeSnapshot) bool {
	return a.Count != b.Count || a.Latest != b.Latest || !a.DBMod.Equal(b.DBMod) || !a.WALMod.Equal(b.WALMod)
}

func snapshotStore(dbPath string) (storeSnapshot, error) {
	db, err := openDB(dbPath)
	if err != nil {
		return storeSnapshot{}, err
	}
	defer db.Close()
	var count int
	var latest sql.NullFloat64
	if err := db.QueryRow("SELECT count(*), max(ZDATE) FROM ZCLOUDRECORDING").Scan(&count, &latest); err != nil {
		return storeSnapshot{}, err
	}
	s := storeSnapshot{Count: count}
	if latest.Valid {
		s.Latest = latest.Float64
		s.LatestAt = coreDataToTime(latest.Float64)
	}
	if info, err := os.Stat(dbPath); err == nil {
		s.DBMod = info.ModTime()
	}
	if info, err := os.Stat(dbPath + "-wal"); err == nil {
		s.WALMod = info.ModTime()
	}
	return s, nil
}

func processIDs(name string) ([]int, error) {
	out, err := exec.Command("pgrep", "-x", name).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	var ids []int
	for _, line := range strings.Fields(string(out)) {
		id, err := strconv.Atoi(line)
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func realSyncHooks(dbPath string) syncHooks {
	var beforePIDs = map[int]bool{}
	var ownedPIDs []int
	return syncHooks{
		Snapshot: func() (storeSnapshot, error) { return snapshotStore(dbPath) },
		KickDaemon: func() error {
			target := fmt.Sprintf("gui/%d/com.apple.voicememod", os.Getuid())
			return exec.Command("launchctl", "kickstart", target).Run()
		},
		AppRunning: func() (bool, error) {
			ids, err := processIDs("VoiceMemos")
			if err != nil {
				return false, err
			}
			for _, id := range ids {
				beforePIDs[id] = true
			}
			return len(ids) > 0, nil
		},
		LaunchHidden: func() error {
			app := "/System/Applications/VoiceMemos.app"
			if _, err := os.Stat(app); err != nil {
				return err
			}
			if err := exec.Command("open", "-gj", app).Run(); err != nil {
				return err
			}
			time.Sleep(500 * time.Millisecond)
			ids, err := processIDs("VoiceMemos")
			if err != nil {
				return err
			}
			for _, id := range ids {
				if !beforePIDs[id] {
					ownedPIDs = append(ownedPIDs, id)
				}
			}
			return nil
		},
		QuitLaunchedApp: func() error {
			var errs []string
			for _, id := range ownedPIDs {
				p, err := os.FindProcess(id)
				if err != nil {
					errs = append(errs, err.Error())
					continue
				}
				if err := p.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
					errs = append(errs, err.Error())
				}
			}
			if len(errs) > 0 {
				return errors.New(strings.Join(errs, "; "))
			}
			return nil
		},
		Sleep: time.Sleep,
	}
}

func defaultSyncOptions() syncOptions {
	return syncOptions{DaemonWait: 3 * time.Second, AppWait: 12 * time.Second, PollInterval: 500 * time.Millisecond}
}

func runSync(ctx context.Context) (syncResult, error) {
	dbPath, _ := resolvePaths()
	if _, err := os.Stat(dbPath); err != nil {
		return syncResult{}, err
	}
	return syncStore(ctx, defaultSyncOptions(), realSyncHooks(filepath.Clean(dbPath)))
}
