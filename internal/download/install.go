package download

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"

	"ggosu/internal/chart"
	_ "ggosu/internal/chart/bms"
	_ "ggosu/internal/chart/ojn"
	_ "ggosu/internal/chart/osu"
	_ "ggosu/internal/chart/sm"
)

const maxPackage = int64(1 << 30)
const maxExtracted = uint64(2 << 30)
const maxFiles = 20000

// These files are supplied by gameplay's built-in osu hitsound fallback.
var defaultHit = regexp.MustCompile(`^(normal|soft|drum)-hit(normal|whistle|finish|clap)[0-9]*\.wav$`)

func (l *Library) install(ctx context.Context, o offer) (result Result, err error) {
	if !sourceID(o.Source, o.ID) {
		return result, fmt.Errorf("잘못된 곡 식별자입니다")
	}
	name := o.Source + "-" + o.ID
	destination := filepath.Join(l.root, name)
	if info, e := os.Lstat(destination); e == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return result, fmt.Errorf("설치 위치에 다른 파일이 있습니다")
		}
		r, e := readResult(filepath.Join(destination, ".ggosu-install.json"))
		if e != nil || r.Source != o.Source || r.ID != o.ID {
			return result, fmt.Errorf("기존 폴더를 덮어쓸 수 없습니다: %s", name)
		}
		checked, e := verify(ctx, destination)
		if e != nil {
			return result, fmt.Errorf("기존 설치 확인 실패: %w", e)
		}
		r.Charts, r.Hashes, r.Already = checked.Charts, checked.Hashes, true
		return r, nil
	} else if !os.IsNotExist(e) {
		return result, e
	}
	if err := os.MkdirAll(l.root, 0755); err != nil {
		return result, err
	}
	root, err := os.OpenRoot(l.root)
	if err != nil {
		return result, err
	}
	defer root.Close()
	stage, err := os.MkdirTemp(l.root, ".install-*")
	if err != nil {
		return result, err
	}
	stageName := filepath.Base(stage)
	// Root confines cleanup and publication to the selected installation root.
	defer func() {
		if e := root.RemoveAll(stageName); e != nil {
			err = errors.Join(err, fmt.Errorf("설치 임시 파일 정리: %w", e))
		}
	}()
	archivePath := filepath.Join(stage, "package.zip")
	resp, err := l.get(ctx, o.url)
	if err != nil {
		return result, err
	}
	if resp.ContentLength > maxPackage {
		resp.Body.Close()
		return result, fmt.Errorf("다운로드 묶음이 1 GiB를 넘습니다")
	}
	f, err := os.Create(archivePath)
	if err != nil {
		resp.Body.Close()
		return result, err
	}
	var received int64
	total := max(resp.ContentLength, 0)
	buf := make([]byte, 128<<10)
	for {
		if err = ctx.Err(); err != nil {
			break
		}
		var n int
		n, err = resp.Body.Read(buf)
		if n > 0 {
			received += int64(n)
			if received > maxPackage {
				err = fmt.Errorf("다운로드 묶음이 1 GiB를 넘습니다")
				break
			}
			if _, e := f.Write(buf[:n]); e != nil {
				err = e
				break
			}
			l.update("download", received, total, 0)
		}
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			break
		}
	}
	err = errors.Join(err, resp.Body.Close(), f.Close())
	if err != nil {
		return result, err
	}
	content := filepath.Join(stage, "content")
	if err := os.Mkdir(content, 0755); err != nil {
		return result, err
	}
	l.update("extract", received, total, 0)
	if err := l.extract(ctx, archivePath, content, received, total); err != nil {
		return result, err
	}
	l.update("verify", received, total, l.State().Files)
	checked, err := verify(ctx, content)
	if err != nil {
		return result, err
	}
	result = Result{Source: o.Source, ID: o.ID, Title: o.Title, Charts: checked.Charts, Hashes: checked.Hashes}
	manifest, err := os.Create(filepath.Join(content, ".ggosu-install.json"))
	if err != nil {
		return result, err
	}
	err = json.NewEncoder(manifest).Encode(result)
	if err == nil {
		err = manifest.Sync()
	}
	if err := errors.Join(err, manifest.Close()); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := root.Rename(filepath.Join(stageName, "content"), name); err != nil {
		return result, fmt.Errorf("곡 설치 반영: %w", err)
	}
	return result, nil
}

func (l *Library) extract(ctx context.Context, archivePath, content string, received, total int64) (err error) {
	z, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("ZIP·OSZ·SMZIP 묶음이 아니거나 파일이 손상됐습니다: %w", err)
	}
	defer z.Close()
	if len(z.File) > maxFiles {
		return fmt.Errorf("묶음의 파일 수가 %d개를 넘습니다", maxFiles)
	}
	root, err := os.OpenRoot(content)
	if err != nil {
		return err
	}
	defer root.Close()
	var size uint64
	seen := map[string]bool{}
	files := 0
	for _, file := range z.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name, err := zipName(file.Name)
		if err != nil {
			return err
		}
		if file.Mode()&os.ModeSymlink != 0 || !file.FileInfo().IsDir() && !file.Mode().IsRegular() {
			return fmt.Errorf("특수 파일은 설치할 수 없습니다: %s", name)
		}
		if file.FileInfo().IsDir() {
			if err := root.MkdirAll(name, 0755); err != nil {
				return err
			}
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("묶음 안에 중복된 파일 경로가 있습니다: %s", name)
		}
		seen[key] = true
		if file.UncompressedSize64 > maxExtracted-size {
			return fmt.Errorf("압축 해제 크기가 2 GiB를 넘습니다")
		}
		size += file.UncompressedSize64
		// Chart packages are data. Executables and command files never enter songs.
		if executable(name) || name == ".ggosu-install.json" {
			continue
		}
		if err := root.MkdirAll(filepath.FromSlash(path.Dir(name)), 0755); err != nil {
			return err
		}
		out, err := root.OpenFile(filepath.FromSlash(name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			out.Close()
			return err
		}
		n, copyErr := copyContext(ctx, out, io.LimitReader(in, int64(file.UncompressedSize64)+1))
		closeErr := errors.Join(in.Close(), out.Close())
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
		if uint64(n) != file.UncompressedSize64 {
			return fmt.Errorf("압축 파일 크기가 일치하지 않습니다: %s", name)
		}
		files++
		l.update("extract", received, total, files)
	}
	return nil
}

func zipName(name string) (string, error) {
	if !utf8.ValidString(name) {
		var err error
		name, err = japanese.ShiftJIS.NewDecoder().String(name)
		if err != nil {
			return "", fmt.Errorf("압축 파일 이름을 읽을 수 없습니다")
		}
	}
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, ":\x00") {
		return "", fmt.Errorf("안전하지 않은 압축 경로입니다")
	}
	for _, p := range strings.Split(name, "/") {
		if p == ".." {
			return "", fmt.Errorf("상위 폴더로 벗어나는 압축 경로입니다")
		}
	}
	name = path.Clean(name)
	if name == "." || !filepath.IsLocal(filepath.FromSlash(name)) {
		return "", fmt.Errorf("안전하지 않은 압축 경로입니다")
	}
	return name, nil
}

func executable(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".exe", ".dll", ".com", ".bat", ".cmd", ".ps1", ".msi", ".lnk", ".url", ".vbs", ".scr", ".sh":
		return true
	}
	return false
}

func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 128<<10)
	var copied int64
	for {
		if err := ctx.Err(); err != nil {
			return copied, err
		}
		n, err := src.Read(buf)
		if n > 0 {
			w, e := dst.Write(buf[:n])
			copied += int64(w)
			if e != nil {
				return copied, e
			}
			if w != n {
				return copied, io.ErrShortWrite
			}
		}
		if err == io.EOF {
			return copied, nil
		}
		if err != nil {
			return copied, err
		}
	}
}

func verify(ctx context.Context, root string) (Result, error) {
	var result Result
	var problems []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !chart.Supported(p) {
			return nil
		}
		charts, err := chart.Load(p)
		if err != nil {
			problems = append(problems, err.Error())
			return nil
		}
		for _, c := range charts {
			if len(c.Notes) == 0 {
				continue
			}
			valid := true
			for _, a := range c.Assets {
				if a.Kind != "audio" && a.Kind != "bank" {
					continue
				}
				if _, err := chart.AssetPath(p, a.Path); err != nil {
					if os.IsNotExist(err) && defaultHit.MatchString(filepath.Base(a.Path)) {
						continue
					}
					problems = append(problems, err.Error())
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			hash, err := c.Digest()
			if err != nil {
				problems = append(problems, err.Error())
				continue
			}
			result.Charts++
			result.Hashes = append(result.Hashes, hash)
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if result.Charts == 0 {
		if len(problems) > 0 {
			return result, fmt.Errorf("설치할 수 있는 채보·음원이 없습니다: %s", problems[0])
		}
		return result, fmt.Errorf("ggosu에서 플레이할 수 있는 채보가 없습니다")
	}
	return result, nil
}
