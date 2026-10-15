package archive

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

type linkKind int

const (
	linkDirect linkKind = iota
	linkPage
	linkUnsupported
)

var fileExt = map[string]bool{
	".zip": true, ".rar": true, ".7z": true, ".lzh": true, ".lha": true, ".osz": true,
	".bms": true, ".bme": true, ".bml": true, ".pms": true, ".bmson": true,
}

// unsupportedHosts need a browser, an account or client-side decryption.
var unsupportedHosts = []string{
	"mega.nz", "mega.co.nz", "onedrive.live.com", "1drv.ms", "skydrive.live.com", "mediafire.com",
	"axfc.net", "bowlroll.net", "getuploader.com", "firestorage.jp", "box.com", "pixiv.net",
}

var driveFile = regexp.MustCompile(`/file/d/([A-Za-z0-9_-]+)`)
var driveFolder = regexp.MustCompile(`/drive/(?:u/\d+/)?folders/([A-Za-z0-9_-]+)`)
var archiveURL = regexp.MustCompile(`^/web/(\d+)[a-z_]*/(.+)$`)

// classify maps a link to a direct file URL, a page to scan, or a reason it cannot be fetched.
func classify(raw string) (linkKind, string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return linkUnsupported, "not a web link"
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	for _, h := range unsupportedHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return linkUnsupported, h
		}
	}
	switch host {
	case "drive.google.com", "docs.google.com":
		id := u.Query().Get("id")
		if m := driveFile.FindStringSubmatch(u.Path); m != nil {
			id = m[1]
		}
		if m := driveFolder.FindStringSubmatch(u.Path); m != nil {
			// The embedded view lists a public folder's files as plain links.
			return linkPage, "https://drive.google.com/embeddedfolderview?id=" + m[1]
		}
		if id == "" {
			return linkUnsupported, "google drive link without file id"
		}
		return linkDirect, "https://drive.usercontent.google.com/download?id=" + url.QueryEscape(id) + "&export=download&confirm=t"
	case "drive.usercontent.google.com":
		return linkDirect, u.String()
	case "dropbox.com", "dl.dropbox.com", "dl.dropboxusercontent.com":
		q := u.Query()
		q.Del("raw")
		q.Set("dl", "1")
		u.RawQuery = q.Encode()
		return linkDirect, u.String()
	case "web.archive.org":
		if m := archiveURL.FindStringSubmatch(u.EscapedPath()); m != nil && fileExt[strings.ToLower(path.Ext(m[2]))] {
			return linkDirect, "https://web.archive.org/web/" + m[1] + "id_/" + m[2]
		}
		return linkPage, u.String()
	}
	if fileExt[strings.ToLower(path.Ext(u.Path))] {
		return linkDirect, u.String()
	}
	return linkPage, u.String()
}

// maxPageFiles separates a song page (one archive, maybe ogg and wav) from an uploader or author index.
const maxPageFiles = 3

var hrefRE = regexp.MustCompile(`(?i)href\s*=\s*["']?([^"'\s>]+)`)
var bareURL = regexp.MustCompile(`https?://[^"'<>\s\\]+`)

// Resolve turns an item link into direct file URLs, scanning one page deep for download links.
func Resolve(ctx context.Context, c *Client, raw string) ([]string, error) {
	kind, v := classify(raw)
	switch kind {
	case linkDirect:
		return []string{v}, nil
	case linkUnsupported:
		return nil, fmt.Errorf("unsupported link (%s): %s", v, raw)
	}
	html, err := c.Text(ctx, v)
	if err != nil {
		return nil, err
	}
	found := pageLinks(v, html)
	var direct []string
	var blocked []string
	for _, l := range found {
		switch k, d := classify(l); k {
		case linkDirect:
			direct = appendUnique(direct, d)
		case linkUnsupported:
			blocked = appendUnique(blocked, d)
		}
	}
	if len(direct) == 0 {
		if len(blocked) > 0 {
			return nil, fmt.Errorf("only unsupported links (%s) on %s", strings.Join(blocked, ", "), raw)
		}
		return nil, fmt.Errorf("no download link on %s", raw)
	}
	if len(direct) > maxPageFiles {
		return nil, fmt.Errorf("page lists %d files, pick one manually: %s", len(direct), raw)
	}
	return direct, nil
}

// pageLinks lists absolute links on a page in order, skipping media and the page itself.
func pageLinks(base, html string) []string {
	var out []string
	add := func(ref string) {
		ref = strings.ReplaceAll(ref, "&amp;", "&")
		abs, err := resolveRef(base, ref)
		if err != nil || abs == base || !strings.HasPrefix(abs, "http") {
			return
		}
		if k, _ := classify(abs); k == linkPage {
			return
		}
		out = appendUnique(out, abs)
	}
	for _, m := range hrefRE.FindAllStringSubmatch(html, -1) {
		add(m[1])
	}
	for _, m := range bareURL.FindAllString(html, -1) {
		add(m)
	}
	return out
}
