package archive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
)

// ErrTooLarge marks a file larger than the caller's per-file limit.
var ErrTooLarge = errors.New("file too large")

// Client spaces requests per host and retries throttled or failed requests.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	Interval  time.Duration // minimum gap between requests to one host
	Retries   int
	Log       func(format string, args ...any)

	mu   sync.Mutex
	next map[string]time.Time
}

func NewClient(userAgent string, interval time.Duration) *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 0},
		UserAgent: userAgent,
		Interval:  interval,
		Retries:   3,
		Log:       func(string, ...any) {},
		next:      map[string]time.Time{},
	}
}

func (c *Client) wait(ctx context.Context, host string) error {
	c.mu.Lock()
	now := time.Now()
	at := c.next[host]
	if at.Before(now) {
		at = now
	}
	c.next[host] = at.Add(c.Interval)
	c.mu.Unlock()
	t := time.NewTimer(time.Until(at))
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// get sends a GET, retrying network errors, 429 and 5xx. Callers close the body.
func (c *Client) get(ctx context.Context, raw string, header http.Header) (*http.Response, error) {
	var last error
	for try := 0; try <= c.Retries; try++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range header {
			req.Header[k] = v
		}
		req.Header.Set("User-Agent", c.UserAgent)
		if err := c.wait(ctx, req.URL.Host); err != nil {
			return nil, err
		}
		resp, err := c.HTTP.Do(req)
		backoff := time.Duration(1<<try) * 2 * time.Second
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if dns := (*net.DNSError)(nil); errors.As(err, &dns) && dns.IsNotFound {
				return nil, err // dead host, retrying will not help
			}
			last = err
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			last = fmt.Errorf("GET %s: %s", raw, resp.Status)
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 && s < 600 {
				backoff = time.Duration(s) * time.Second
			}
			resp.Body.Close()
		default:
			return resp, nil
		}
		if try < c.Retries {
			c.Log("retry in %s: %v", backoff, last)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return nil, last
}

func (c *Client) body(ctx context.Context, raw string, header http.Header, limit int64) ([]byte, error) {
	resp, err := c.get(ctx, raw, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", raw, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("GET %s: response over %d bytes", raw, limit)
	}
	return b, nil
}

func (c *Client) JSON(ctx context.Context, raw string, header http.Header, v any) error {
	b, err := c.body(ctx, raw, header, 256<<20)
	if err != nil {
		return err
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("GET %s: %w", raw, err)
	}
	return nil
}

// Text returns a page as UTF-8, decoding Shift_JIS pages used by older BMS sites.
func (c *Client) Text(ctx context.Context, raw string) (string, error) {
	b, err := c.body(ctx, raw, nil, 32<<20)
	if err != nil {
		return "", err
	}
	return decodeText(b), nil
}

func decodeText(b []byte) string {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if utf8.Valid(b) {
		return string(b)
	}
	if d, err := japanese.ShiftJIS.NewDecoder().Bytes(b); err == nil {
		return string(d)
	}
	return string(b)
}

// Download saves raw into dir, resuming dir/.part, and returns the final file name.
// Unless text is allowed, only archive files are accepted so error pages never land in the archive.
func (c *Client) Download(ctx context.Context, raw, dir string, maxSize int64, text bool) (name string, size int64, sum string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, "", err
	}
	part := filepath.Join(dir, ".part")
	var off int64
	if st, err := os.Stat(part); err == nil {
		off = st.Size()
	}
	header := http.Header{}
	if off > 0 {
		header.Set("Range", fmt.Sprintf("bytes=%d-", off))
	}
	resp, err := c.get(ctx, raw, header)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	switch {
	case resp.StatusCode == http.StatusPartialContent && off > 0 && strings.HasPrefix(resp.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-", off)):
		flags |= os.O_APPEND
	case resp.StatusCode == http.StatusOK:
		flags |= os.O_TRUNC
		off = 0
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		return "", 0, "", errors.Join(fmt.Errorf("GET %s: stale partial file", raw), os.Remove(part))
	default:
		return "", 0, "", fmt.Errorf("GET %s: %s", raw, resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		return "", 0, "", fmt.Errorf("GET %s: got a web page, not a file", raw)
	}
	if maxSize > 0 && resp.ContentLength > 0 && off+resp.ContentLength > maxSize {
		return "", 0, "", fmt.Errorf("%w: %d bytes", ErrTooLarge, off+resp.ContentLength)
	}
	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return "", 0, "", err
	}
	var src io.Reader = resp.Body
	if maxSize > 0 {
		src = io.LimitReader(resp.Body, maxSize-off+1)
	}
	n, copyErr := io.Copy(f, src)
	if err := errors.Join(copyErr, f.Close()); err != nil {
		return "", 0, "", err
	}
	size = off + n
	if maxSize > 0 && size > maxSize {
		return "", 0, "", errors.Join(fmt.Errorf("%w: over %d bytes", ErrTooLarge, maxSize), os.Remove(part))
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return "", 0, "", fmt.Errorf("GET %s: short body %d of %d", raw, n, resp.ContentLength)
	}
	head, sum, err := hashFile(part)
	if err != nil {
		return "", 0, "", err
	}
	if !text && !isArchive(head) {
		return "", 0, "", errors.Join(fmt.Errorf("GET %s: not an archive", raw), os.Remove(part))
	}
	name = fileName(resp)
	if err := os.Rename(part, filepath.Join(dir, name)); err != nil {
		return "", 0, "", err
	}
	return name, size, sum, nil
}

func hashFile(p string) (head []byte, sum string, err error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	head = make([]byte, 8)
	k, _ := io.ReadFull(f, head)
	head = head[:k]
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, "", err
	}
	return head, hex.EncodeToString(h.Sum(nil)), nil
}

func isArchive(b []byte) bool {
	return bytes.HasPrefix(b, []byte("PK\x03\x04")) ||
		bytes.HasPrefix(b, []byte("Rar!")) ||
		bytes.HasPrefix(b, []byte("7z\xbc\xaf")) ||
		len(b) >= 5 && b[2] == '-' && b[3] == 'l' && (b[4] == 'h' || b[4] == 'z') // LHA
}

// fileName picks the server's file name, falling back to the URL path, and keeps it inside the item directory.
func fileName(resp *http.Response) string {
	name := ""
	if _, p, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil {
		name = p["filename"]
	}
	if name == "" {
		if s, err := url.PathUnescape(path.Base(resp.Request.URL.Path)); err == nil {
			name = s
		}
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	name = strings.Trim(name, ". ")
	if name == "" || name == "." || name == ".part" {
		name = "download"
	}
	return name
}
