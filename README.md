# キミコミ新連載検出システム

[キミコミ](https://kimicomi.com)の曜日別連載一覧ページを毎日1回取得し、前回取得時点からシリーズが増減した場合にSlackへ通知するAWS Lambdaアプリケーションです。

## 機能

- 曜日別連載ページ（月〜日、その他）から全シリーズ情報を取得
- シリーズID、URL、タイトルを抽出
- 前回データとの差分を検出（新規追加・削除）
- Slack Bot API経由で変更を通知
- S3に最新データを保存

## 技術スタック

- Go 1.24
- AWS Lambda
- AWS S3
- Slack Bot API
- golang.org/x/net/html (HTMLパーサー)

## 設定

### ローカル環境

`config.json`ファイルを作成してください：

```bash
cp config.json.example config.json
```

`config.json`を編集して必要な値を設定：

```json
{
  "S3BucketName": "your-s3-bucket-name",
  "S3Region": "ap-northeast-1",
  "S3Directory": "kimicomi",
  "S3SeriesKey": "series.json",
  "SlackBotToken": "xoxb-YOUR-SLACK-BOT-TOKEN",
  "SlackChannel": "YOUR_SLACK_CHANNEL_ID"
}
```

### Lambda環境

Lambda環境では環境変数から設定を読み込みます：

- `S3_BUCKET_NAME`: シリーズデータを保存するS3バケット名
- `S3_REGION`: AWSリージョン（例: `ap-northeast-1`）
- `S3_DIRECTORY`: S3内のディレクトリ名（例: `kimicomi`）
- `S3_SERIES_KEY`: シリーズデータのファイル名（例: `series.json`）
- `SLACK_BOT_TOKEN`: Slack Bot Token（`xoxb-`で始まる）
- `SLACK_CHANNEL`: 通知先SlackチャンネルID

### 設定項目

- `S3BucketName`/`S3_BUCKET_NAME`: シリーズデータを保存するS3バケット名
- `S3Region`/`S3_REGION`: AWSリージョン（例: `ap-northeast-1`）
- `S3Directory`/`S3_DIRECTORY`: S3内のディレクトリ名
- `S3SeriesKey`/`S3_SERIES_KEY`: シリーズデータのファイル名
- `SlackBotToken`/`SLACK_BOT_TOKEN`: Slack Bot Token
- `SlackChannel`/`SLACK_CHANNEL`: 通知先SlackチャンネルID

## ローカル実行

```bash
go run main.go
```

実行すると以下の処理が行われます：
1. 8つの曜日別ページから連載情報を取得
2. S3から前回データを読み込み
3. 差分を検出して変更があればSlackに通知
4. 最新データをS3に保存

## ビルド

Lambda用にビルド：

```bash
GOOS=linux GOARCH=amd64 go build -o bootstrap main.go
zip function.zip bootstrap
```

## デプロイ

### デプロイスクリプトを使用

デプロイスクリプトが提供されています：

```bash
# デフォルト設定でデプロイ
./deploy.sh

# プロファイルと関数名を指定してデプロイ
./deploy.sh [profile-name] [function-name]
```

デフォルトでは `lambda-deploy` プロファイルと `kimicomi-series-checker` 関数名が使用されます。

### 手動デプロイ

1. AWS Lambdaで関数を作成（`provided.al2023`ランタイム）
2. `function.zip`をアップロード
3. 環境変数を設定（Lambda環境の設定項目を参照）
4. EventBridgeで1日1回実行するルールを作成（例: `cron(0 0 * * ? *)`）
5. Lambda実行ロールに必要なIAMポリシーを付与

## 必要なIAMポリシー

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject"
      ],
      "Resource": "arn:aws:s3:::YOUR_BUCKET_NAME/kimicomi/*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "arn:aws:logs:*:*:*"
    }
  ]
}
```

## 実装の特徴

- **環境変数対応**: Lambda環境では環境変数から設定を読み込み
- **User-Agent設定**: キミコミサイトへのアクセス時に適切なUser-Agentヘッダーを設定
- **URLエンコード**: 日本語パラメータを適切にエンコード
- **HTMLパース**: `golang.org/x/net/html`を使用した堅牢なHTMLパース
- **エラーハンドリング**: エラー発生時はSlackに通知、詳細なログ出力
- **重複排除**: 複数の曜日に重複して掲載されているシリーズを自動的に統合
- **デプロイ自動化**: デプロイスクリプトによる簡単なデプロイ

## データ形式

S3に保存されるJSON形式：

```json
{
  "series": [
    {
      "id": "96e54ba774eb7",
      "url": "https://kimicomi.com/series/96e54ba774eb7",
      "title": "魔王の始め方　THE COMIC"
    }
  ]
}
```

## Slack通知形式

新規連載や削除された連載がある場合、以下の形式で通知されます：

```
キミコミ連載情報の変更を検出しました

*【新規連載】*
* <https://kimicomi.com/series/xxx|作品タイトル>

*【削除された連載】*
* <https://kimicomi.com/series/yyy|作品タイトル>
```
