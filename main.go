package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

var urlRE = regexp.MustCompile(`https?://[^\s"'<>]*userapi\.com[^\s"'<>]*`)

var mediaExtensions = map[string]map[string]bool{
	"photo": {".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true},
	"voice": {".ogg": true, ".mp3": true, ".m4a": true, ".aac": true},
	"doc":   {".pdf": true, ".doc": true, ".docx": true, ".zip": true, ".txt": true, ".epub": true},
}

// ---- logger ----

const (
	lvlDebug = iota
	lvlInfo
	lvlWarn
	lvlError
)

type appLogger struct {
	inner *log.Logger
	level int
}

func newAppLogger(w io.Writer, levelStr string) *appLogger {
	lvl := lvlInfo
	switch strings.ToUpper(strings.TrimSpace(levelStr)) {
	case "DEBUG":
		lvl = lvlDebug
	case "WARN", "WARNING":
		lvl = lvlWarn
	case "ERROR":
		lvl = lvlError
	}
	return &appLogger{inner: log.New(w, "", log.Ltime), level: lvl}
}

func (l *appLogger) debugf(format string, args ...any) {
	if l.level <= lvlDebug {
		l.inner.Printf("DEBUG "+format, args...)
	}
}

func (l *appLogger) infof(format string, args ...any) {
	if l.level <= lvlInfo {
		l.inner.Printf("INFO  "+format, args...)
	}
}

func (l *appLogger) warnf(format string, args ...any) {
	if l.level <= lvlWarn {
		l.inner.Printf("WARN  "+format, args...)
	}
}

func (l *appLogger) errorf(format string, args ...any) {
	if l.level <= lvlError {
		l.inner.Printf("ERROR "+format, args...)
	}
}

// ---- stats ----

type stats struct {
	chatsSeen       atomic.Int64
	chatsWithMedia  atomic.Int64
	urlsFound       atomic.Int64
	downloaded      atomic.Int64
	skippedExisting atomic.Int64
	failed          atomic.Int64
	mu              sync.Mutex
	failedURLs      []string
}

func (s *stats) recordFail(u string) {
	s.failed.Add(1)
	s.mu.Lock()
	s.failedURLs = append(s.failedURLs, u)
	s.mu.Unlock()
}

// ---- config ----

type config struct {
	archive   string
	threshold float64
	jitter    float64
	workers   int
	timeout   float64
	userAgent string
	chat      string
	dryRun    bool
	force     bool
	types     string
	logLevel  string
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		lower := strings.ToLower(strings.TrimSpace(v))
		return lower == "1" || lower == "true" || lower == "yes"
	}
	return def
}

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.archive, "archive", envStr("ARCHIVE_PATH", "/workspace/Archive"), "path to Archive folder")
	flag.Float64Var(&cfg.threshold, "threshold", envFloat("THRESHOLD_SECONDS", 1.0), "base delay between downloads (seconds)")
	flag.Float64Var(&cfg.jitter, "jitter", envFloat("JITTER_SECONDS", 0.5), "max random jitter added to threshold")
	flag.IntVar(&cfg.workers, "workers", envInt("WORKERS", 3), "parallel download workers")
	flag.Float64Var(&cfg.timeout, "timeout", envFloat("HTTP_TIMEOUT", 30), "HTTP timeout (seconds)")
	flag.StringVar(&cfg.userAgent, "user-agent", envStr("USER_AGENT", defaultUserAgent), "HTTP User-Agent")
	flag.StringVar(&cfg.chat, "chat", envStr("CHAT_FILTER", ""), "only process this chat id")
	flag.BoolVar(&cfg.dryRun, "dry-run", false, "scan and report URLs without downloading")
	flag.BoolVar(&cfg.force, "force", envBool("FORCE", false), "re-download existing files")
	flag.StringVar(&cfg.types, "types", envStr("MEDIA_TYPES", "all"), "media types: photo,voice,doc,all")
	flag.StringVar(&cfg.logLevel, "log-level", envStr("LOG_LEVEL", "INFO"), "log level: DEBUG,INFO,WARN,ERROR")
	flag.Parse()
	return cfg
}

// ---- helpers ----

func urlMediaType(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "other"
	}
	ext := strings.ToLower(path.Ext(u.Path))
	for typeName, exts := range mediaExtensions {
		if exts[ext] {
			return typeName
		}
	}
	return "other"
}

func allowedTypes(typesArg string) map[string]bool {
	if strings.TrimSpace(strings.ToLower(typesArg)) == "all" {
		return nil
	}
	result := make(map[string]bool)
	for _, t := range strings.Split(typesArg, ",") {
		result[strings.TrimSpace(strings.ToLower(t))] = true
	}
	return result
}

func filenameFor(rawURL string) string {
	digest := fmt.Sprintf("%x", sha1.Sum([]byte(rawURL)))[:8]
	u, _ := url.Parse(rawURL)
	last := path.Base(u.Path)
	if last == "" || last == "." || last == "/" {
		last = "file"
	}
	// impg/* paths are JPEG photos served without an extension
	if !strings.Contains(last, ".") {
		if strings.Contains(u.Path, "/impg/") || strings.Contains(u.RawQuery, "type=album") {
			last += ".jpg"
		}
	}
	return digest + "_" + last
}

func extractURLs(htmlPath string) ([]string, error) {
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		return nil, err
	}
	// VK dumps are windows-1251; URLs are ASCII so byte-level regex is fine
	matches := urlRE.FindAllString(string(data), -1)
	seen := make(map[string]bool, len(matches))
	var result []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			result = append(result, m)
		}
	}
	return result, nil
}

// ---- scan ----

type downloadTask struct {
	chatID string
	url    string
	dest   string
}

type scanResult struct {
	hasHTML   bool
	hasMedia  bool
	totalURLs int
	skipped   int
	tasks     []downloadTask
}

func scanChat(chatDir string, cfg config, typesFilter map[string]bool) (*scanResult, error) {
	entries, err := os.ReadDir(chatDir)
	if err != nil {
		return nil, err
	}

	var htmlFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "messages") && strings.HasSuffix(e.Name(), ".html") {
			htmlFiles = append(htmlFiles, filepath.Join(chatDir, e.Name()))
		}
	}
	sort.Strings(htmlFiles)

	res := &scanResult{hasHTML: len(htmlFiles) > 0}
	if !res.hasHTML {
		return res, nil
	}

	urlSet := make(map[string]bool)
	for _, hf := range htmlFiles {
		urls, err := extractURLs(hf)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", hf, err)
		}
		for _, u := range urls {
			urlSet[u] = true
		}
	}

	if typesFilter != nil {
		filtered := make(map[string]bool)
		for u := range urlSet {
			if typesFilter[urlMediaType(u)] {
				filtered[u] = true
			}
		}
		urlSet = filtered
	}

	if len(urlSet) == 0 {
		return res, nil
	}

	res.hasMedia = true
	mediaDir := filepath.Join(chatDir, "media")

	urls := make([]string, 0, len(urlSet))
	for u := range urlSet {
		urls = append(urls, u)
	}
	sort.Strings(urls)

	for _, u := range urls {
		dest := filepath.Join(mediaDir, filenameFor(u))
		if !cfg.force {
			if fi, statErr := os.Stat(dest); statErr == nil && fi.Size() > 0 {
				res.skipped++
				continue
			}
		}
		res.tasks = append(res.tasks, downloadTask{
			chatID: filepath.Base(chatDir),
			url:    u,
			dest:   dest,
		})
	}
	res.totalURLs = len(res.tasks) + res.skipped
	return res, nil
}

// ---- worker ----

func runWorker(client *http.Client, task downloadTask, cfg config, st *stats, logger *appLogger) {
	if err := os.MkdirAll(filepath.Dir(task.dest), 0755); err != nil {
		logger.warnf("[%s] FAIL %s -> mkdir: %v", task.chatID, filepath.Base(task.dest), err)
		st.recordFail(task.url)
		return
	}

	req, err := http.NewRequest("GET", task.url, nil)
	if err != nil {
		logger.warnf("[%s] FAIL %s -> %v", task.chatID, filepath.Base(task.dest), err)
		st.recordFail(task.url)
		return
	}
	req.Header.Set("User-Agent", cfg.userAgent)

	resp, err := client.Do(req)
	if err != nil {
		logger.warnf("[%s] FAIL %s -> %v", task.chatID, filepath.Base(task.dest), err)
		st.recordFail(task.url)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.warnf("[%s] FAIL %s -> HTTP %d", task.chatID, filepath.Base(task.dest), resp.StatusCode)
		st.recordFail(task.url)
		return
	}

	tmp := task.dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		logger.warnf("[%s] FAIL %s -> %v", task.chatID, filepath.Base(task.dest), err)
		st.recordFail(task.url)
		return
	}

	size, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		logger.warnf("[%s] FAIL %s -> %v", task.chatID, filepath.Base(task.dest), err)
		st.recordFail(task.url)
		return
	}

	if err := os.Rename(tmp, task.dest); err != nil {
		os.Remove(tmp)
		logger.warnf("[%s] FAIL %s -> rename: %v", task.chatID, filepath.Base(task.dest), err)
		st.recordFail(task.url)
		return
	}

	st.downloaded.Add(1)
	logger.infof("[%s] OK   %s (%d bytes)", task.chatID, filepath.Base(task.dest), size)
}

// ---- main ----

func main() {
	cfg := parseConfig()

	ts := time.Now().Format("20060102_150405")
	_ = os.MkdirAll(cfg.archive, 0755)
	lf, err := os.OpenFile(
		filepath.Join(cfg.archive, "download_"+ts+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open log file: %v\n", err)
		os.Exit(1)
	}
	defer lf.Close()

	logger := newAppLogger(io.MultiWriter(os.Stderr, lf), cfg.logLevel)

	messagesRoot := filepath.Join(cfg.archive, "messages")
	if fi, err := os.Stat(messagesRoot); err != nil || !fi.IsDir() {
		logger.errorf("messages/ not found under %s", cfg.archive)
		os.Exit(2)
	}

	entries, err := os.ReadDir(messagesRoot)
	if err != nil {
		logger.errorf("reading %s: %v", messagesRoot, err)
		os.Exit(2)
	}

	var chatDirs []string
	for _, e := range entries {
		if e.IsDir() {
			chatDirs = append(chatDirs, filepath.Join(messagesRoot, e.Name()))
		}
	}
	sort.Strings(chatDirs)

	if cfg.chat != "" {
		var filtered []string
		for _, d := range chatDirs {
			if filepath.Base(d) == cfg.chat {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) == 0 {
			logger.errorf("chat %s not found in %s", cfg.chat, messagesRoot)
			os.Exit(2)
		}
		chatDirs = filtered
	}

	logger.infof(
		"scanning %d chat(s) from %s (threshold=%.2fs jitter=%.2fs workers=%d dry_run=%v force=%v types=%s)",
		len(chatDirs), messagesRoot, cfg.threshold, cfg.jitter, cfg.workers, cfg.dryRun, cfg.force, cfg.types,
	)

	typesFilter := allowedTypes(cfg.types)
	st := &stats{}

	// Phase 1: scan all chats in parallel → collect tasks
	type scanItem struct {
		dir string
		res *scanResult
		err error
	}
	resultCh := make(chan scanItem, len(chatDirs))
	var scanWg sync.WaitGroup
	for _, d := range chatDirs {
		scanWg.Add(1)
		go func(dir string) {
			defer scanWg.Done()
			res, err := scanChat(dir, cfg, typesFilter)
			resultCh <- scanItem{dir: dir, res: res, err: err}
		}(d)
	}
	scanWg.Wait()
	close(resultCh)

	var allTasks []downloadTask
	for item := range resultCh {
		chatID := filepath.Base(item.dir)
		if item.err != nil {
			logger.warnf("[%s] scan error: %v", chatID, item.err)
			continue
		}
		if !item.res.hasHTML {
			logger.debugf("[%s] no messages*.html, skipping", chatID)
			continue
		}
		st.chatsSeen.Add(1)
		if !item.res.hasMedia {
			logger.infof("[%s] no media URLs", chatID)
			continue
		}
		st.chatsWithMedia.Add(1)
		st.urlsFound.Add(int64(item.res.totalURLs))
		st.skippedExisting.Add(int64(item.res.skipped))
		logger.infof("[%s] urls=%d new=%d skipped=%d",
			chatID, item.res.totalURLs, len(item.res.tasks), item.res.skipped)
		allTasks = append(allTasks, item.res.tasks...)
	}

	if cfg.dryRun {
		logger.infof(
			"dry-run: chats_seen=%d with_media=%d urls_found=%d skipped_existing=%d would_download=%d",
			st.chatsSeen.Load(), st.chatsWithMedia.Load(), st.urlsFound.Load(),
			st.skippedExisting.Load(), len(allTasks),
		)
		return
	}

	// Phase 2: feed tasks into channel, N workers consume
	if len(allTasks) > 0 {
		taskCh := make(chan downloadTask, len(allTasks))
		for _, t := range allTasks {
			taskCh <- t
		}
		close(taskCh)

		client := &http.Client{
			Timeout: time.Duration(float64(time.Second) * cfg.timeout),
		}

		numWorkers := cfg.workers
		if numWorkers < 1 {
			numWorkers = 1
		}

		var wg sync.WaitGroup
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)*997))
				for task := range taskCh {
					runWorker(client, task, cfg, st, logger)
					delay := cfg.threshold + rng.Float64()*cfg.jitter
					time.Sleep(time.Duration(float64(time.Second) * delay))
				}
			}(i)
		}
		wg.Wait()
	}

	logger.infof(
		"done: chats_seen=%d with_media=%d urls_found=%d downloaded=%d skipped_existing=%d failed=%d",
		st.chatsSeen.Load(), st.chatsWithMedia.Load(), st.urlsFound.Load(),
		st.downloaded.Load(), st.skippedExisting.Load(), st.failed.Load(),
	)
}
