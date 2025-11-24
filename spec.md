# キミコミ新連載検出システム 仕様書

## 概要

キミコミ（https://kimicomi.com）の曜日別連載一覧ページを毎日 1 回取得し、  
前回取得時点からシリーズが増減した場合に Slack へ通知するシステムを構築する。  
AWS Lambda（Go）、EventBridge、S3 を利用する。

## 目的

- 新規連載（シリーズ追加）を自動検出する  
- 連載終了・非公開化などによる減少（シリーズ削除）も通知する  

## 全体フロー

1. EventBridge が 1 日 1 回 Lambda を起動  
2. Lambda が以下ページを取得  
   - https://kimicomi.com/category/manga?type=連載中&day=月  
   - https://kimicomi.com/category/manga?type=連載中&day=火  
   - https://kimicomi.com/category/manga?type=連載中&day=水  
   - https://kimicomi.com/category/manga?type=連載中&day=木  
   - https://kimicomi.com/category/manga?type=連載中&day=金  
   - https://kimicomi.com/category/manga?type=連載中&day=土  
   - https://kimicomi.com/category/manga?type=連載中&day=日  
   - https://kimicomi.com/category/manga?type=連載中&day=その他  
3. HTML 内の `/series/<id>` 形式のリンクを抽出  
4. 全シリーズ IDと作品のリンク、作品タイトル をセットに統合  
5. S3 の前回データと比較し、増減差分を算出  
6. 差分があれば対象シリーズページを取得しタイトルを抽出  
7. Slack へ通知  
8. 最新 IDと作品のリンク、作品タイトル を S3 に保存