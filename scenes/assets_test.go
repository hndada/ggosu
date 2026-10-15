package scenes

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A module missing from the embed list fails the whole import graph, so every
// absolute import in the screen scripts must be served by Assets.
func TestImportsAreEmbedded(t *testing.T) {
	imports := regexp.MustCompile(`(?m)^import\b[^'"]*['"](/[^'"]+\.js)['"]`)
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".test.js") || strings.HasPrefix(p, "wailsjs") {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range imports.FindAllStringSubmatch(string(src), -1) {
			if _, err := fs.Stat(Assets, strings.TrimPrefix(m[1], "/")); err != nil {
				t.Errorf("%s imports %s, which is not embedded", p, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLocalFontsAreEmbedded(t *testing.T) {
	for _, name := range []string{"gmarketmedium.woff2", "gmarketbold.woff2", "counter.woff2", "gmarket-license.txt", "chakra-license.txt"} {
		b, err := fs.ReadFile(Assets, "ui/fonts/"+name)
		if err != nil || len(b) == 0 {
			t.Fatalf("missing local font or notice %s: %v", name, err)
		}
		if strings.HasSuffix(name, ".woff2") && string(b[:4]) != "wOF2" {
			t.Fatalf("%s is not WOFF2", name)
		}
	}
}
