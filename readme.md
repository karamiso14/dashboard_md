# Markdown Viewer

指定したフォルダ内の Markdown をブラウザで読むための、Go製ローカルサーバーです。

## できること

- フォルダ内の `.md` ファイルをサイドバーに一覧表示
- `README.md` / `readme.md` / `index.md` を起点として自動表示
- Markdown内の相対 `.md` リンクをビューア内で遷移
- 相対パスで書かれた画像・音声・動画を表示
- GitHub Flavored Markdown（テーブル、取り消し線、タスクリストなど）をサポート
- 指定フォルダの外へアクセスできないように制限

## 起動

Go 1.22以降をインストールして、プロジェクトのフォルダで以下を実行します。

```powershell
go mod tidy
go run . -dir .
```

ブラウザで `http://127.0.0.1:8080` を開いてください。

表示対象を別のフォルダにする場合:

```powershell
go run . -dir C:\path\to\markdown-folder
```

ポートを変える場合:

```powershell
go run . -dir . -addr 127.0.0.1:3000
```

## リンクの書き方

同じフォルダまたは子フォルダのMarkdownへ、通常どおり相対リンクを書けます。

```md
[セットアップ手順](docs/setup.md)
![画面例](images/screenshot.png)
```

`setup.md` を開いているときは、`[次へ](next.md)` のようなリンクも正しく解決されます。

[セットアップ手順](docs/setup.md)