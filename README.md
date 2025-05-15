# mysql-restore

大容量SQLファイルをMySQLへ安全にリストアできるGo製CLIツールです。

## 特徴
- 100MB超のSQLファイルも分割投入で安定リストア
- タイムアウトや切断時も自動リトライ
- TUIで進捗・ログ・一時停止/再開が可能
- 特定行からの再開も対応
- クロスプラットフォーム対応（macOS/Linux/Windows）

## インストール
GitHub Releasesから各OS用バイナリをダウンロードしてください。

## 使い方
```sh
./mysql-restore --host <host> --port <port> --user <user> [--password <password>] [--db <dbname>] --file <file.sql> [--resume-line <n>]
```

### オプション
- `--host` : MySQLホスト（デフォルト: localhost）
- `--port` : ポート番号（デフォルト: 3306）
- `--user` : ユーザー名（デフォルト: root）
- `--password` : パスワード（省略可）
- `--db` : データベース名（省略可）
- `--file` : リストアするSQLファイルパス
- `--resume-line` : 指定行から再開（デフォルト: 1）

### TUI操作
- `p` : 一時停止
- `r` : 再開
- 完了後は自動でプロセス終了

## GitHub Actionsによる自動リリース
タグをpushすると各OS/アーキテクチャ向けバイナリを自動ビルドし、リリースに添付します。

## ライセンス
MIT License
