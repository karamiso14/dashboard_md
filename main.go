package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

const (
	defaultMarkdownDir = "./md"
	defaultAddress     = "0.0.0.0:8080"
	maxSearchResults   = 20
	maxSearchSnippets  = 2
	maxSnippetRunes    = 100
	maxRecentFiles     = 5
)

// ページのHTMLを埋め込むことで、Goサーバーを単一バイナリのまま保ちつつ、
// ページのマークアップ・スタイル・ブラウザ側の処理を個別に編集しやすくする。
//
//go:embed page.html
var pageHTML string

var pageTemplate = template.Must(template.New("page").Parse(pageHTML))

type pageData struct {
	Title       string
	Root        string
	Path        string
	Navigation  template.HTML
	Breadcrumb  template.HTML
	Content     template.HTML
	Portal      bool
	RecentFiles []recentFile
}

type viewer struct {
	root string
	md   goldmark.Markdown
}

type navFile struct {
	name string
	rel  string
}

type navFolder struct {
	name, rel string
	folders   map[string]*navFolder
	files     []navFile
}
type searchResult struct {
	Title   string   `json:"title"`
	Path    string   `json:"path"`
	Matches []string `json:"matches"`
}

// recentFile はトップページに表示する、最近更新されたファイルの情報。
type recentFile struct {
	Title     string
	Path      string
	ViewPath  string
	UpdatedAt string
	modTime   time.Time
}

func main() {
	defaultDir := os.Getenv("MARKDOWN_DIR")
	if defaultDir == "" {
		defaultDir = defaultMarkdownDir
	}

	dir := flag.String("dir", defaultDir, "Markdown folder to serve")
	addr := flag.String("addr", defaultAddress, "address to listen on")
	flag.Parse()

	root, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		log.Fatalf("invalid directory: %s", root)
	}
	v := newViewer(root)
	mux := http.NewServeMux()
	mux.HandleFunc("/", v.handle)
	log.Printf("Markdown Viewer: http://%s  (folder: %s)", *addr, root)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// newViewer はすべてのドキュメントで使う Markdown レンダラーの設定をまとめる。
func newViewer(root string) *viewer {
	return &viewer{root: root, md: goldmark.New(goldmark.WithExtensions(extension.GFM))}
}

// handle はアプリケーションが公開する少数のルートにリクエストを振り分ける。
func (v *viewer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/view/"+url.PathEscape(v.defaultDocument()), http.StatusFound)
		return
	}
	if r.URL.Path == "/api/search" {
		v.search(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/raw/") {
		v.serveRaw(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/view/") {
		v.serveDocument(w, r)
		return
	}
	http.NotFound(w, r)
}

// search は Markdown ファイルを走査し、ファイル名の一致と本文の短い抜粋を返す。
func (v *viewer) search(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	results := make([]searchResult, 0)
	if query != "" {
		filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".md") {
				return nil
			}
			rel, err := filepath.Rel(v.root, p)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			source, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			matches := searchSnippets(string(source), query)
			if strings.Contains(strings.ToLower(path.Base(rel)), query) || len(matches) > 0 {
				results = append(results, searchResult{Title: titleFrom(rel), Path: rel, Matches: matches})
			}
			return nil
		})
	}
	if len(results) > maxSearchResults {
		results = results[:maxSearchResults]
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(results)
}

// searchSnippets は表示用に整形した、最初に一致した空でない行を返す。
func searchSnippets(source, query string) []string {
	matches := make([]string, 0, maxSearchSnippets)
	for _, line := range strings.Split(source, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" || !strings.Contains(strings.ToLower(line), query) {
			continue
		}
		matches = append(matches, truncateRunes(line, maxSnippetRunes))
		if len(matches) == maxSearchSnippets {
			break
		}
	}
	return matches
}

// truncateRunes は表示用の抜粋でマルチバイト文字が途中で切れないようにする。
func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}

// safePath はリクエストのパスをデコードし、Markdown のルート外へ出られないようにする。
func (v *viewer) safePath(encoded string) (string, string, error) {
	p, err := url.PathUnescape(encoded)
	if err != nil {
		return "", "", err
	}
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))
	if p == "/" || strings.Contains(p, "\\") {
		return "", "", errors.New("invalid path")
	}
	rel := strings.TrimPrefix(p, "/")
	full := filepath.Join(v.root, filepath.FromSlash(rel))
	check, err := filepath.Rel(v.root, full)
	if err != nil || check == ".." || strings.HasPrefix(check, ".."+string(filepath.Separator)) {
		return "", "", errors.New("outside root")
	}
	return rel, full, nil
}

// serveRaw は Markdown ドキュメントから相対パスで参照されたメディアを配信する。
func (v *viewer) serveRaw(w http.ResponseWriter, r *http.Request) {
	_, full, err := v.safePath(strings.TrimPrefix(r.URL.Path, "/raw/"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, full)
}

// serveDocument は Markdown ファイルを共通のページテンプレートへ描画する。
func (v *viewer) serveDocument(w http.ResponseWriter, r *http.Request) {
	rel, full, err := v.safePath(strings.TrimPrefix(r.URL.Path, "/view/"))
	if err != nil || !strings.EqualFold(filepath.Ext(rel), ".md") {
		http.NotFound(w, r)
		return
	}
	source, err := os.ReadFile(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var rendered bytes.Buffer
	if err := v.md.Convert(source, &rendered); err != nil {
		http.Error(w, "could not render Markdown", http.StatusInternalServerError)
		return
	}
	isPortal := strings.EqualFold(path.Base(rel), "index.md")
	data := pageData{
		Title:      titleFrom(rel),
		Root:       filepath.Base(v.root),
		Path:       filepath.ToSlash(rel),
		Navigation: v.navigation(rel),
		Breadcrumb: v.breadcrumb(rel),
		Content:    template.HTML(rendered.String()),
		Portal:     isPortal,
	}
	if isPortal {
		data.RecentFiles = v.recentFiles()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplate.Execute(w, data); err != nil {
		log.Println(err)
	}
}

// recentFiles は更新日時の新しい Markdown ファイルを、表示上限まで返す。
func (v *viewer) recentFiles() []recentFile {
	files := make([]recentFile, 0, maxRecentFiles)
	filepath.WalkDir(v.root, func(fullPath string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(fullPath), ".md") {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(v.root, fullPath)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		files = append(files, recentFile{
			Title:     titleFrom(rel),
			Path:      rel,
			ViewPath:  "/view/" + url.PathEscape(rel),
			UpdatedAt: info.ModTime().Format("2006/01/02 15:04"),
			modTime:   info.ModTime(),
		})
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].Path < files[j].Path
		}
		return files[i].modTime.After(files[j].modTime)
	})
	if len(files) > maxRecentFiles {
		files = files[:maxRecentFiles]
	}
	return files
}

// defaultDocument は index.md を優先し、なければ最初の Markdown ファイルを使う。
func (v *viewer) defaultDocument() string {
	for _, name := range []string{"index.md", "INDEX.md"} {
		if _, err := os.Stat(filepath.Join(v.root, name)); err == nil {
			return name
		}
	}
	var first string
	filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".md") && first == "" {
			first, _ = filepath.Rel(v.root, p)
		}
		return nil
	})
	return filepath.ToSlash(first)
}

// navigation は現在のページの描画に必要な階層ファイルツリーを一度だけ組み立てる。
func (v *viewer) navigation(current string) template.HTML {
	root := &navFolder{folders: make(map[string]*navFolder)}
	filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".md") {
			return nil
		}
		rel, err := filepath.Rel(v.root, p)
		if err != nil {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		folder := root
		for i, part := range parts[:len(parts)-1] {
			if folder.folders[part] == nil {
				folder.folders[part] = &navFolder{name: part, rel: strings.Join(parts[:i+1], "/"), folders: make(map[string]*navFolder)}
			}
			folder = folder.folders[part]
		}
		folder.files = append(folder.files, navFile{name: parts[len(parts)-1], rel: strings.Join(parts, "/")})
		return nil
	})
	var b strings.Builder
	v.renderFolder(&b, root, current)
	return template.HTML(b.String())
}

// renderFolder はエスケープ済みのナビゲーションラベルを出力する。パスはサーバー側で生成する。
func (v *viewer) renderFolder(b *strings.Builder, folder *navFolder, current string) {
	b.WriteString("<ul>")
	names := make([]string, 0, len(folder.folders))
	for name := range folder.folders {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		child := folder.folders[name]
		open := ""
		if strings.HasPrefix(current, child.rel+"/") {
			open = " open"
		}
		fmt.Fprintf(b, `<li><details%s><summary>%s</summary>`, open, template.HTMLEscapeString(child.name))
		v.renderFolder(b, child, current)
		b.WriteString("</details></li>")
	}
	sort.Slice(folder.files, func(i, j int) bool { return folder.files[i].name < folder.files[j].name })
	for _, file := range folder.files {
		class := ""
		if file.rel == current {
			class = ` class="active"`
		}
		fmt.Fprintf(b, `<li><a%s href="/view/%s">%s</a></li>`, class, url.PathEscape(file.rel), template.HTMLEscapeString(file.name))
	}
	b.WriteString("</ul>")
}

// breadcrumb はファイルシステム上のパスではなく、意図して相対ファイルパスを表示する。
func (v *viewer) breadcrumb(rel string) template.HTML {
	parts := strings.Split(rel, "/")
	var b strings.Builder
	b.WriteString(`<a href="/">Home</a> / `)
	for i, part := range parts {
		if i > 0 {
			b.WriteString(" / ")
		}
		b.WriteString(template.HTMLEscapeString(part))
	}
	return template.HTML(b.String())
}
func titleFrom(rel string) string { return strings.TrimSuffix(path.Base(rel), path.Ext(rel)) }
