curl http://localhost:11435/api/chat -d '{
  "model": "qwen3.5:0.8b",
  "messages": [
    {
      "role": "user",
      "content": "Tell me everything you know about weights and biases."
    }
  ],
  "stream": false,
  "think": false
}'
