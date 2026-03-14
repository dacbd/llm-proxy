#!/bin/bash
prompt=$(cat <<EOF
I live next to a car wash that is one block away. My car is dirty and I want to clean it. How should I get to the car wash?
EOF
)

response=$(
jq -n \
  --arg model "qwen3.5:0.8b" \
  --arg prompt "$prompt" \
  '{
    model: $model,
    prompt: $prompt,
    stream: false,
    think: false
  }' \
| curl --silent http://localhost:11435/api/generate \
  -H "Content-Type: application/json" \
  --data-binary @-
)

jq -r '.response' <<<"$response"

