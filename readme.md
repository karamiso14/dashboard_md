# Markdown Viewer

Markdownフォルダをブラウザで閲覧するための、Go製ローカルサーバーです。Dockerで起動し、個人作業用のポータルとして利用できます。

## 主な機能

- `index.md` をトップページとして表示
- Markdownファイルをサイドバーのフォルダツリーに表示
- フォルダツリーを折りたたみ・展開
- 相対 `.md` リンクをビューア内で遷移
- 相対パスで指定した画像・音声・動画を表示
- GitHub Flavored Markdown（テーブル、タスクリスト、取り消し線など）に対応
- 指定したMarkdownフォルダ外へのアクセスを防止

## 必要なもの

- Docker Desktop

## 起動

プロジェクト直下で実行します。

```powershell
docker compose down
docker compose up --build
```

ブラウザで [http://localhost:8080](http://localhost:8080) を開きます。

バックグラウンドで起動する場合は、次のようにします。

```powershell
docker compose up --build -d
```

停止する場合:

```powershell
docker compose down
```

## 表示するMarkdownフォルダの設定

標準では、このプロジェクト内の `md` フォルダを表示します。別PCや別の保管場所にあるMarkdownフォルダを使う場合は、設定ファイルを作成します。

```powershell
Copy-Item .env.example .env
```

`.env` の `MARKDOWN_SOURCE_DIR` に、表示したいMarkdownフォルダを設定してください。

```ini
# プロジェクト直下の md フォルダを使う場合
MARKDOWN_SOURCE_DIR=./md

# プロジェクトと同じ階層にある my-notes フォルダを使う場合
# MARKDOWN_SOURCE_DIR=../my-notes
```

設定を変更した後は、`docker compose down` と `docker compose up --build` を実行して再起動します。指定したフォルダはコンテナに読み取り専用でマウントされます。

## Markdownの構成例

`index.md` がトップページです。相対リンクで他のページへつなげられます。

```text
md/
├── index.md
├── schedule/
│   └── weekly-plan.md
├── tasks/
│   └── now.md
└── knowledge/
    └── development-notes.md
```

```md
# 個人作業ポータル

- [今週の予定](schedule/weekly-plan.md)
- [今日やること](tasks/now.md)
- [開発メモ](knowledge/development-notes.md)
```

画像も同様に相対パスで参照できます。

```md
![画面例](images/screenshot.png)
```
