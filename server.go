package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/gzhttp"
)

//go:embed dist
var distFS embed.FS

const (
	maxProxySize = 500 * 1024 * 1024 // 500MB max proxy download
	httpTimeout  = 60 * time.Second
)

var (
	port          int
	login         string
	password      string
	cdn           string
	downloadDir   string
	download      bool
	downloadCache bool
	workers       int

	// Shared HTTP client with timeout
	httpClient = &http.Client{
		Timeout: httpTimeout,
	}

	// Track files being downloaded to prevent race conditions
	activeDownloads sync.Map
)

func init() {
	flag.IntVar(&port, "port", 8000, "Server port")
	flag.StringVar(&login, "login", "", "HTTP Basic Auth username")
	flag.StringVar(&password, "password", "", "HTTP Basic Auth password")
	flag.StringVar(&cdn, "cdn", "https://cdn.dos.zone/vcsky/", "CDN base URL")
	flag.StringVar(&downloadDir, "dir", "", "Asset cache directory (defaults to OS temp folder)")
	flag.BoolVar(&download, "download", false, "Download all assets and exit")
	flag.BoolVar(&downloadCache, "download-cache", false, "Download all assets to cache in the background")
	flag.IntVar(&workers, "workers", 8, "Number of parallel download workers")
}

func loadEnvConfig() {
	// Override with environment variables if set
	if v := os.Getenv("PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			port = n
		}
	}
	if v := os.Getenv("AUTH_LOGIN"); v != "" {
		login = v
	}
	if v := os.Getenv("AUTH_PASSWORD"); v != "" {
		password = v
	}
	if v := os.Getenv("CDN"); v != "" {
		cdn = v
	}
	if v := os.Getenv("DOWNLOAD_DIR"); v != "" {
		downloadDir = v
	}
	if v := os.Getenv("DOWNLOAD_CACHE"); v == "1" || v == "true" {
		downloadCache = true
	}
	if v := os.Getenv("WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			workers = n
		}
	}
}

func main() {
	flag.Parse()
	loadEnvConfig()

	// Default to OS temp folder if not set
	if downloadDir == "" {
		downloadDir = filepath.Join(os.TempDir(), "reVCDOS")
	}

	// Validate cache directory is usable
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		log.Fatalf("Cannot create cache directory %s: %v", downloadDir, err)
	}

	// Download and exit
	if download {
		if err := doDownloadAssets(); err != nil {
			log.Fatalf("Failed to download assets: %v", err)
		}
		if err := doDownloadAudio(); err != nil {
			log.Fatalf("Failed to download audio: %v", err)
		}
		log.Println("Download complete")
		return
	}

	// Download cache in background
	if downloadCache {
		go func() {
			log.Println("Starting background cache download...")
			if err := doDownloadAssets(); err != nil {
				log.Printf("Warning: Failed to download assets: %v", err)
			}
			if err := doDownloadAudio(); err != nil {
				log.Printf("Warning: Failed to download audio: %v", err)
			}
			log.Println("Background cache download complete")
		}()
	}

	mux := http.NewServeMux()

	// Proxy routes
	mux.HandleFunc("/vcsky/", handleVcsky)

	// Index route
	mux.HandleFunc("/", handleRoot)

	// Apply middleware
	var handler http.Handler = mux
	if login != "" && password != "" {
		handler = basicAuthMiddleware(handler)
	}
	handler = gzhttp.GzipHandler(handler)
	handler = corsHeadersMiddleware(handler)
	handler = loggingMiddleware(handler)

	fmt.Printf("Starting server on http://localhost:%d\n", port)
	fmt.Printf("cdn: %s\n", cdn)
	fmt.Printf("cache: %s\n", downloadDir)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), handler))
}

// Middleware

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.wroteHeader = true
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		encoding := ""
		switch rw.Header().Get("Content-Encoding") {
		case "br":
			encoding = " br"
		case "gzip":
			encoding = " gzip"
		}
		log.Printf("%s %s %s %d%s", r.RemoteAddr, r.Method, r.URL.Path, rw.status, encoding)
	})
}

func corsHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		next.ServeHTTP(w, r)
	})
}

func basicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			unauthorized(w)
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "basic" {
			unauthorized(w)
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			unauthorized(w)
			return
		}

		creds := strings.SplitN(string(decoded), ":", 2)
		if len(creds) != 2 {
			unauthorized(w)
			return
		}

		if subtle.ConstantTimeCompare([]byte(creds[0]), []byte(login)) != 1 ||
			subtle.ConstantTimeCompare([]byte(creds[1]), []byte(password)) != 1 {
			unauthorized(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// Handlers

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		serveIndex(w, r)
		return
	}

	// Strip leading slash for safe path handling
	cleanedPath := strings.TrimPrefix(r.URL.Path, "/")

	// Validate path to prevent directory traversal
	if strings.Contains(cleanedPath, "..") {
		log.Printf("Path traversal attempt blocked: %s", r.URL.Path)
		http.NotFound(w, r)
		return
	}

	// Serve static files from embedded dist/
	filePath := "dist/" + cleanedPath
	data, err := distFS.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Set content type
	contentType := mime.TypeByExtension(filepath.Ext(cleanedPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Write(data)
}

func serveIndex(w http.ResponseWriter, _ *http.Request) {
	content, err := distFS.ReadFile("dist/index.html")
	if err != nil {
		log.Printf("Error reading index.html: %v", err)
		http.Error(w, "index.html not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(content); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func handleVcsky(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/vcsky/")

	// Validate path to prevent directory traversal
	if strings.Contains(path, "..") {
		log.Printf("Path traversal attempt blocked: %s", r.URL.Path)
		http.NotFound(w, r)
		return
	}

	localPath := filepath.Join(downloadDir, path)

	// Try local file first
	if serveLocalFile(w, r, localPath) {
		return
	}

	// Proxy to CDN
	url := cdn + path
	if r.URL.RawQuery != "" {
		url += "?" + r.URL.RawQuery
	}

	log.Printf("fetch %s%s", cdn, path)
	proxyAndCache(w, r, url, localPath)
}

// File serving and proxying

func serveLocalFile(w http.ResponseWriter, r *http.Request, localPath string) bool {
	acceptEncoding := strings.ToLower(r.Header.Get("Accept-Encoding"))
	clientAcceptsBr := strings.Contains(acceptEncoding, "br")
	clientAcceptsGzip := strings.Contains(acceptEncoding, "gzip")

	// Determine base path and check for compressed versions
	basePath := strings.TrimSuffix(strings.TrimSuffix(localPath, ".br"), ".gz")
	isBrFile := strings.HasSuffix(localPath, ".br")
	isGzFile := strings.HasSuffix(localPath, ".gz")

	// Try to serve pre-compressed versions if they exist
	var servePath string
	var encoding string

	if clientAcceptsBr {
		brPath := basePath + ".br"
		if info, err := os.Stat(brPath); err == nil && !info.IsDir() {
			servePath = brPath
			encoding = "br"
		}
	}
	if servePath == "" && clientAcceptsGzip {
		gzPath := basePath + ".gz"
		if info, err := os.Stat(gzPath); err == nil && !info.IsDir() {
			servePath = gzPath
			encoding = "gzip"
		}
	}
	if servePath == "" {
		// Try the original requested path
		info, err := os.Stat(localPath)
		if err != nil {
			if os.IsNotExist(err) {
				return false
			}
			log.Printf("Error accessing file %s: %v", localPath, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return true
		}
		if info.IsDir() {
			return false
		}
		servePath = localPath
		if isBrFile {
			encoding = "br"
		} else if isGzFile {
			encoding = "gzip"
		}
	}

	// Set content type based on the uncompressed filename
	contentType := mime.TypeByExtension(filepath.Ext(basePath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if strings.HasSuffix(basePath, ".wasm") {
		contentType = "application/wasm"
	}
	w.Header().Set("Content-Type", contentType)

	// Handle brotli decompression if client doesn't support it
	if encoding == "br" && !clientAcceptsBr {
		file, err := os.Open(servePath)
		if err != nil {
			log.Printf("Error opening file %s: %v", servePath, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return true // Handled (with error)
		}
		defer file.Close()

		reader := brotli.NewReader(file)
		if _, err := io.Copy(w, reader); err != nil {
			log.Printf("Error decompressing file %s: %v", servePath, err)
		}
		return true
	}

	// Set encoding header if serving compressed file
	if encoding != "" {
		w.Header().Set("Content-Encoding", encoding)
	}

	http.ServeFile(w, r, servePath)
	return true
}

func proxyAndCache(w http.ResponseWriter, r *http.Request, url string, localPath string) {
	// Try local first
	if serveLocalFile(w, r, localPath) {
		return
	}

	// Check if file is being downloaded by background goroutine
	_, alreadyDownloading := activeDownloads.Load(localPath)

	// Use context for cancellation
	ctx, cancel := context.WithTimeout(r.Context(), httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, r.Method, url, nil)
	if err != nil {
		log.Printf("Error creating proxy request to %s: %v", url, err)
		http.Error(w, "Proxy error: failed to create request", http.StatusBadGateway)
		return
	}

	for k, v := range r.Header {
		if strings.ToLower(k) != "host" && strings.ToLower(k) != "accept-encoding" {
			req.Header[k] = v
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("Proxy request to %s failed: %v", url, err)
		http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Check content length for size limit
	if resp.ContentLength > maxProxySize {
		log.Printf("Proxy response from %s too large: %d bytes", url, resp.ContentLength)
		http.Error(w, "Response too large", http.StatusBadGateway)
		return
	}

	// Limit actual bytes read to prevent chunked encoding bypass
	limitedBody := io.LimitReader(resp.Body, maxProxySize)

	// Copy response headers
	for k, v := range resp.Header {
		kLower := strings.ToLower(k)
		if kLower != "transfer-encoding" && kLower != "connection" && kLower != "content-security-policy" {
			w.Header()[k] = v
		}
	}

	isBrFile := strings.HasSuffix(url, ".br")
	clientAcceptsBr := strings.Contains(strings.ToLower(r.Header.Get("Accept-Encoding")), "br")
	needDecompress := isBrFile && !clientAcceptsBr

	if needDecompress {
		w.Header().Del("Content-Encoding")
		w.Header().Del("Content-Length")
	}

	// Don't cache non-200 responses
	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		if needDecompress {
			reader := brotli.NewReader(limitedBody)
			if _, err := io.Copy(w, reader); err != nil {
				log.Printf("Error decompressing proxy response from %s: %v", url, err)
			}
		} else {
			if _, err := io.Copy(w, limitedBody); err != nil {
				log.Printf("Error copying proxy response from %s: %v", url, err)
			}
		}
		return
	}

	// Skip caching if background download is in progress
	if alreadyDownloading {
		w.WriteHeader(resp.StatusCode)
		if needDecompress {
			reader := brotli.NewReader(limitedBody)
			if _, err := io.Copy(w, reader); err != nil {
				log.Printf("Error decompressing proxy response: %v", err)
			}
		} else {
			if _, err := io.Copy(w, limitedBody); err != nil {
				log.Printf("Error copying proxy response: %v", err)
			}
		}
		return
	}

	// Create cache directory
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		log.Printf("Warning: failed to create cache directory for %s: %v", localPath, err)
		// Continue without caching
		w.WriteHeader(resp.StatusCode)
		if needDecompress {
			reader := brotli.NewReader(limitedBody)
			if _, err := io.Copy(w, reader); err != nil {
				log.Printf("Error decompressing proxy response: %v", err)
			}
		} else {
			if _, err := io.Copy(w, limitedBody); err != nil {
				log.Printf("Error copying proxy response: %v", err)
			}
		}
		return
	}

	// Create temp file for caching
	tempFile, err := os.CreateTemp(filepath.Dir(localPath), ".tmp-*")
	if err != nil {
		log.Printf("Warning: cannot create temp file for caching %s: %v (proxying without cache)", localPath, err)
		w.WriteHeader(resp.StatusCode)
		if needDecompress {
			reader := brotli.NewReader(limitedBody)
			if _, err := io.Copy(w, reader); err != nil {
				log.Printf("Error decompressing proxy response: %v", err)
			}
		} else {
			if _, err := io.Copy(w, limitedBody); err != nil {
				log.Printf("Error copying proxy response: %v", err)
			}
		}
		return
	}
	tempFileName := tempFile.Name()

	w.WriteHeader(resp.StatusCode)

	// Use TeeReader to write to cache while reading
	// This correctly handles the stream splitting
	teeReader := io.TeeReader(limitedBody, tempFile)

	var copyErr error
	if needDecompress {
		// Decompress the tee'd stream for the client
		brReader := brotli.NewReader(teeReader)
		_, copyErr = io.Copy(w, brReader)
	} else {
		// Send raw data to client (tee already writes to cache)
		_, copyErr = io.Copy(w, teeReader)
	}

	tempFile.Close()

	if copyErr != nil {
		log.Printf("Error during proxy/cache of %s: %v", url, copyErr)
		os.Remove(tempFileName)
		return
	}

	// Atomically move temp file to final location
	if err := os.Rename(tempFileName, localPath); err != nil {
		log.Printf("Error caching file %s: %v", localPath, err)
		os.Remove(tempFileName)
	}
}

// Download functions

func downloadFile(url, destPath string) error {
	// Skip if file exists
	if _, err := os.Stat(destPath); err == nil {
		return nil
	}

	// Skip if already being downloaded
	if _, loaded := activeDownloads.LoadOrStore(destPath, true); loaded {
		return nil
	}
	defer activeDownloads.Delete(destPath)

	// Double-check file doesn't exist (another goroutine may have finished)
	if _, err := os.Stat(destPath); err == nil {
		return nil
	}

	// Create directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept-Encoding", "gzip, br")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	// Decompress if needed
	var body io.Reader = resp.Body
	switch resp.Header.Get("Content-Encoding") {
	case "br":
		body = brotli.NewReader(resp.Body)
	case "gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return err
		}
		defer gr.Close()
		body = gr
	}

	// Write to temp file first
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, body)
	out.Close()
	if err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, destPath)
}

func doDownloadAssets() error {
	log.Println("=== Downloading game assets ===")

	data, err := distFS.ReadFile("dist/streaming_files.txt")
	if err != nil {
		return fmt.Errorf("cannot read streaming_files.txt: %w", err)
	}

	var files []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			files = append(files, line)
		}
	}

	log.Printf("Loaded %d files from streaming_files.txt", len(files))

	baseURL := strings.TrimSuffix(cdn, "/") + "/fetched/models/gta3.img"
	outputDir := filepath.Join(downloadDir, "fetched/models/gta3.img")

	return downloadWithWorkers(files, baseURL, outputDir, "assets")
}

func doDownloadAudio() error {
	log.Println("=== Downloading audio files ===")

	// Radio stations
	radioFiles := []string{"kchat.adf", "vcpr.adf", "fever.adf", "vrock.adf", "wave.adf", "emotion.adf", "espant.adf"}
	baseURL := strings.TrimSuffix(cdn, "/") + "/fetched/audio"
	outputDir := filepath.Join(downloadDir, "fetched/audio")

	log.Printf("Downloading %d radio stations...", len(radioFiles))
	if err := downloadWithWorkers(radioFiles, baseURL, outputDir, "radio"); err != nil {
		return err
	}

	// SFX files (0-9940)
	var sfxFiles []string
	for i := 0; i <= 9940; i++ {
		sfxFiles = append(sfxFiles, fmt.Sprintf("%d.mp3", i))
	}

	log.Printf("Downloading %d SFX files...", len(sfxFiles))
	return downloadWithWorkers(sfxFiles, baseURL+"/sfx.raw", filepath.Join(outputDir, "sfx.raw"), "sfx")
}

func downloadWithWorkers(files []string, baseURL, outputDir, label string) error {
	total := len(files)
	var downloaded, skipped, failed atomic.Int64

	// Create work channel
	work := make(chan string, workers)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range work {
				destPath := filepath.Join(outputDir, file)

				// Check if exists
				if _, err := os.Stat(destPath); err == nil {
					skipped.Add(1)
					continue
				}

				url := baseURL + "/" + file
				if err := downloadFile(url, destPath); err != nil {
					failed.Add(1)
					done := downloaded.Load() + skipped.Load() + failed.Load()
					log.Printf("[%s] %d/%d FAIL: %s (%v)", label, done, total, file, err)
				} else {
					downloaded.Add(1)
					done := downloaded.Load() + skipped.Load() + failed.Load()
					log.Printf("[%s] %d/%d OK: %s", label, done, total, file)
				}
			}
		}()
	}

	// Send work
	for _, file := range files {
		work <- file
	}
	close(work)

	// Wait for completion
	wg.Wait()

	failedCount := failed.Load()
	log.Printf("[%s] Done: %d total (downloaded: %d, skipped: %d, failed: %d)",
		label, total, downloaded.Load(), skipped.Load(), failedCount)

	if failedCount > 0 {
		return fmt.Errorf("%d of %d %s files failed to download", failedCount, total, label)
	}

	return nil
}
