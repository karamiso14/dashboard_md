package main

import (
	"bytes"
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
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>{{.Title}} | Markdown Viewer</title><style>
:root{color-scheme:light dark;--bg:#f8fafc;--panel:#fff;--text:#1e293b;--muted:#64748b;--line:#e2e8f0;--accent:#2563eb}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font-family:system-ui,-apple-system,"Segoe UI",sans-serif}.layout{display:grid;grid-template-columns:270px minmax(0,1fr);min-height:100vh}aside{padding:20px;border-right:1px solid var(--line);background:var(--panel)}.brand{display:block;color:var(--text);font-weight:700;text-decoration:none;margin-bottom:18px}.root{font-size:.78rem;color:var(--muted);overflow-wrap:anywhere;margin:0 0 14px}nav ul{list-style:none;padding-left:14px;margin:4px 0}nav>ul{padding-left:0}nav li{margin:3px 0}nav a{display:block;padding:4px 6px;border-radius:5px;color:var(--text);text-decoration:none;overflow-wrap:anywhere}nav a:hover,nav a.active{background:#dbeafe;color:#1d4ed8}main{max-width:980px;width:100%;padding:44px clamp(24px,5vw,72px);margin:0 auto}.crumb{font-size:.86rem;color:var(--muted);margin-bottom:26px;overflow-wrap:anywhere}.crumb a{color:inherit}article{line-height:1.75}article h1,article h2,article h3{line-height:1.25;margin-top:1.65em}article h1{font-size:2em;border-bottom:1px solid var(--line);padding-bottom:.35em}article h2{border-bottom:1px solid var(--line);padding-bottom:.25em}article a{color:var(--accent)}article img{max-width:100%;height:auto}article pre{overflow:auto;padding:16px;background:#0f172a;color:#e2e8f0;border-radius:8px}article code{font-family:ui-monospace,SFMono-Regular,Consolas,monospace}article :not(pre)>code{padding:.15em .35em;background:#e2e8f0;border-radius:4px}article blockquote{border-left:4px solid var(--line);margin-left:0;padding-left:16px;color:var(--muted)}article table{border-collapse:collapse;display:block;overflow:auto}article th,article td{border:1px solid var(--line);padding:7px 10px}@media(max-width:760px){.layout{display:block}aside{border-right:0;border-bottom:1px solid var(--line)}main{padding-top:28px}nav{max-height:220px;overflow:auto}}
.portal{max-width:1100px}.portal>h1{font-size:2.5rem;border:0;margin-top:0}.portal-search{max-width:700px;margin:0 auto 3.5rem;text-align:center}.portal-search label{display:block;font-size:1.05rem;font-weight:650;margin-bottom:12px}.portal-search input{width:100%;padding:18px 22px;border:1px solid var(--line);border-radius:12px;background:var(--panel);color:var(--text);font:inherit;font-size:1.15rem;box-shadow:0 4px 16px rgba(15,23,42,.06)}.portal-search input:focus{outline:3px solid #bfdbfe;border-color:var(--accent)}.search-results{display:grid;gap:8px;text-align:left;margin-top:14px}.search-results a{display:block;padding:12px 14px;border:1px solid var(--line);border-radius:8px;background:var(--panel);color:var(--text);text-decoration:none}.search-results a:hover{border-color:var(--accent)}.search-results small{display:block;color:var(--muted);margin-top:3px}.portal h2{border:0;font-size:1.1rem;letter-spacing:.04em;margin:2.5rem 0 .8rem}.portal h2+ul{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px;list-style:none;padding:0;margin:0}.portal h2+ul li{margin:0}.portal h2+ul a{display:flex;align-items:center;min-height:80px;padding:16px;border:1px solid var(--line);border-radius:10px;background:var(--panel);color:var(--text);font-weight:650;text-decoration:none;box-shadow:0 1px 2px rgba(15,23,42,.04);transition:transform .15s,box-shadow .15s,border-color .15s}.portal h2+ul a:hover{border-color:var(--accent);box-shadow:0 8px 20px rgba(37,99,235,.14);transform:translateY(-2px)}@media(max-width:760px){.portal>h1{font-size:2rem}.portal h2+ul{grid-template-columns:1fr}}
</style></head><body><div class="layout"><aside><a class="brand" href="/">Markdown Viewer</a><p class="root">{{.Root}}</p><nav>{{.Navigation}}</nav></aside><main><div class="crumb">{{.Breadcrumb}}</div><article id="document"{{if .Portal}} class="portal"{{end}}>{{if .Portal}}<section class="portal-search"><label for="search-input">ページを検索</label><input id="search-input" type="search" placeholder="ファイル名・リンク名で検索" autocomplete="off"><div id="search-results" class="search-results" aria-live="polite"></div></section>{{end}}{{.Content}}</article></main></div><script>
const mdPath={{printf "%q" .Path}};
function resolveFromDoc(value){const base='https://viewer.invalid/'+mdPath;return new URL(value,base).pathname.replace(/^\//,'')}
document.querySelectorAll('#document a').forEach(a=>{const href=a.getAttribute('href');if(!href||href.startsWith('#'))return;try{const u=new URL(href,'https://viewer.invalid/'+mdPath);if(u.origin==='https://viewer.invalid'&&/\.md$/i.test(u.pathname)){a.href='/view/'+encodeURIComponent(u.pathname.slice(1)).replace(/%2F/g,'/');if(u.hash)a.href+=u.hash}}catch(_){}});
document.querySelectorAll('#document img, #document video, #document audio, #document source').forEach(el=>{const src=el.getAttribute('src');if(!src||/^(https?:|data:|#)/i.test(src))return;try{el.src='/raw/'+encodeURIComponent(resolveFromDoc(src)).replace(/%2F/g,'/')}catch(_){}});
const searchInput=document.getElementById('search-input');if(searchInput){let searchTimer;const results=document.getElementById('search-results');searchInput.addEventListener('input',()=>{clearTimeout(searchTimer);const query=searchInput.value.trim();if(!query){results.replaceChildren();return}searchTimer=setTimeout(async()=>{try{const response=await fetch('/api/search?q='+encodeURIComponent(query));const items=await response.json();results.replaceChildren(...items.map(item=>{const link=document.createElement('a');link.href='/view/'+encodeURIComponent(item.path).replace(/%2F/g,'/');link.textContent=item.title;const detail=document.createElement('small');detail.textContent=item.path+(item.matches.length?' — '+item.matches.join('、'):'' );link.append(detail);return link}));if(!items.length)results.textContent='該当するページはありません。'}catch(_){results.textContent='検索に失敗しました。'}},180)})}
</script></body></html>`))

type pageData struct { Title, Root, Path string; Navigation, Breadcrumb, Content template.HTML; Portal bool }
type viewer struct { root string; md goldmark.Markdown }
type navFile struct { name, rel string }
type navFolder struct { name, rel string; folders map[string]*navFolder; files []navFile }
type searchResult struct { Title string `json:"title"`; Path string `json:"path"`; Matches []string `json:"matches"` }
var markdownLinkPattern = regexp.MustCompile(`!?\[([^\]]+)\]\([^)]+\)`)

func main() {
	defaultDir := os.Getenv("MARKDOWN_DIR")
	if defaultDir == "" {
		defaultDir = "./md"
	}
	dir := flag.String("dir", defaultDir, "Markdown folder to serve")
	addr := flag.String("addr", "0.0.0.0:8080", "address to listen on")
	flag.Parse()
	root, err := filepath.Abs(*dir); if err != nil { log.Fatal(err) }
	info, err := os.Stat(root); if err != nil || !info.IsDir() { log.Fatalf("invalid directory: %s", root) }
	v := &viewer{root: root, md: goldmark.New(goldmark.WithExtensions(extension.GFM))}
	mux := http.NewServeMux(); mux.HandleFunc("/", v.handle)
	log.Printf("Markdown Viewer: http://%s  (folder: %s)", *addr, root)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
func (v *viewer) handle(w http.ResponseWriter, r *http.Request) { if r.URL.Path == "/" { http.Redirect(w, r, "/view/"+url.PathEscape(v.defaultDocument()), http.StatusFound); return }; if r.URL.Path == "/api/search" { v.search(w, r); return }; if strings.HasPrefix(r.URL.Path, "/raw/") { v.serveRaw(w, r); return }; if strings.HasPrefix(r.URL.Path, "/view/") { v.serveDocument(w, r); return }; http.NotFound(w, r) }
func (v *viewer) search(w http.ResponseWriter, r *http.Request) { query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))); results := make([]searchResult, 0); if query != "" { filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error { if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".md") { return nil }; rel, err := filepath.Rel(v.root, p); if err != nil { return nil }; rel = filepath.ToSlash(rel); source, err := os.ReadFile(p); if err != nil { return nil }; matches := make([]string, 0); for _, link := range markdownLinkPattern.FindAllStringSubmatch(string(source), -1) { if strings.Contains(strings.ToLower(link[1]), query) { matches = append(matches, link[1]) } }; if strings.Contains(strings.ToLower(path.Base(rel)), query) || len(matches) > 0 { results = append(results, searchResult{Title: titleFrom(rel), Path: rel, Matches: matches}) }; return nil }) }; if len(results) > 20 { results = results[:20] }; w.Header().Set("Content-Type", "application/json; charset=utf-8"); json.NewEncoder(w).Encode(results) }
func (v *viewer) safePath(encoded string) (string, string, error) { p, err := url.PathUnescape(encoded); if err != nil { return "", "", err }; p = path.Clean("/" + strings.TrimPrefix(p, "/")); if p == "/" || strings.Contains(p, "\\") { return "", "", errors.New("invalid path") }; rel := strings.TrimPrefix(p, "/"); full := filepath.Join(v.root, filepath.FromSlash(rel)); check, err := filepath.Rel(v.root, full); if err != nil || check == ".." || strings.HasPrefix(check, ".."+string(filepath.Separator)) { return "", "", errors.New("outside root") }; return rel, full, nil }
func (v *viewer) serveRaw(w http.ResponseWriter, r *http.Request) { _, full, err := v.safePath(strings.TrimPrefix(r.URL.Path, "/raw/")); if err != nil { http.NotFound(w, r); return }; http.ServeFile(w, r, full) }
func (v *viewer) serveDocument(w http.ResponseWriter, r *http.Request) { rel, full, err := v.safePath(strings.TrimPrefix(r.URL.Path, "/view/")); if err != nil || !strings.EqualFold(filepath.Ext(rel), ".md") { http.NotFound(w, r); return }; source, err := os.ReadFile(full); if err != nil { http.NotFound(w, r); return }; var rendered bytes.Buffer; if err := v.md.Convert(source, &rendered); err != nil { http.Error(w, "could not render Markdown", http.StatusInternalServerError); return }; w.Header().Set("Content-Type", "text/html; charset=utf-8"); if err := pageTemplate.Execute(w, pageData{Title: titleFrom(rel), Root: filepath.Base(v.root), Path: filepath.ToSlash(rel), Navigation: v.navigation(rel), Breadcrumb: v.breadcrumb(rel), Content: template.HTML(rendered.String()), Portal: strings.EqualFold(path.Base(rel), "index.md")}); err != nil { log.Println(err) } }
func (v *viewer) defaultDocument() string { for _, name := range []string{"index.md", "INDEX.md"} { if _, err := os.Stat(filepath.Join(v.root, name)); err == nil { return name } }; var first string; filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error { if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".md") && first == "" { first, _ = filepath.Rel(v.root, p) }; return nil }); return filepath.ToSlash(first) }
func (v *viewer) navigation(current string) template.HTML {
	root := &navFolder{folders: make(map[string]*navFolder)}
	filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".md") { return nil }
		rel, err := filepath.Rel(v.root, p); if err != nil { return nil }
		parts := strings.Split(filepath.ToSlash(rel), "/")
		folder := root
		for i, part := range parts[:len(parts)-1] {
			if folder.folders[part] == nil { folder.folders[part] = &navFolder{name: part, rel: strings.Join(parts[:i+1], "/"), folders: make(map[string]*navFolder)} }
			folder = folder.folders[part]
		}
		folder.files = append(folder.files, navFile{name: parts[len(parts)-1], rel: strings.Join(parts, "/")})
		return nil
	})
	var b strings.Builder
	v.renderFolder(&b, root, current)
	return template.HTML(b.String())
}
func (v *viewer) renderFolder(b *strings.Builder, folder *navFolder, current string) {
	b.WriteString("<ul>")
	names := make([]string, 0, len(folder.folders)); for name := range folder.folders { names = append(names, name) }; sort.Strings(names)
	for _, name := range names {
		child := folder.folders[name]; open := ""; if strings.HasPrefix(current, child.rel+"/") { open = " open" }
		fmt.Fprintf(b, `<li><details%s><summary>%s</summary>`, open, template.HTMLEscapeString(child.name))
		v.renderFolder(b, child, current)
		b.WriteString("</details></li>")
	}
	sort.Slice(folder.files, func(i, j int) bool { return folder.files[i].name < folder.files[j].name })
	for _, file := range folder.files {
		class := ""; if file.rel == current { class = ` class="active"` }
		fmt.Fprintf(b, `<li><a%s href="/view/%s">%s</a></li>`, class, url.PathEscape(file.rel), template.HTMLEscapeString(file.name))
	}
	b.WriteString("</ul>")
}
func (v *viewer) breadcrumb(rel string) template.HTML { parts := strings.Split(rel, "/"); var b strings.Builder; b.WriteString(`<a href="/">Home</a> / `); for i, part := range parts { if i > 0 { b.WriteString(" / ") }; b.WriteString(template.HTMLEscapeString(part)) }; return template.HTML(b.String()) }
func titleFrom(rel string) string { return strings.TrimSuffix(path.Base(rel), path.Ext(rel)) }
