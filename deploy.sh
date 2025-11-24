#!/bin/bash

# kimicomi-series-checker Lambda デプロイスクリプト

set -e

# デフォルト値の設定
PROFILE=${1:-lambda-deploy}
FUNCTION_NAME=${2:-kimicomi-series-checker}

# AWS CLIのページャーを無効化
export AWS_PAGER=""

echo "Building kimicomi-series-checker for Lambda..."
GOOS=linux GOARCH=amd64 go build -o bootstrap main.go

echo "Creating deployment package..."
zip kimicomi-series-checker.zip bootstrap

echo "Deploying to Lambda..."
echo "Profile: $PROFILE"
echo "Function Name: $FUNCTION_NAME"
aws lambda update-function-code \
    --function-name "$FUNCTION_NAME" \
    --zip-file fileb://kimicomi-series-checker.zip \
    --region ap-northeast-1 \
    --profile "$PROFILE"

echo "Cleaning up..."
rm bootstrap kimicomi-series-checker.zip

echo "Deployment completed successfully!"