# llm-proxy


currently very limited, example usage:
```bash
export WANDB_API_KEY=wandb_v1_xxxxx
export WANDB_PROJECT=wandb/dacbd-proxy
# my strix halo system on tailscale
export OLLAMA_URL=http://100.94.224.71:11434

make run
```

sample:
```bash
make run
go build -o bin/llm-proxy main.go
./bin/llm-proxy run server
time=2026-03-10T18:06:26.227+09:00 level=INFO msg="Weave tracing enabled" project=wandb/dacbd-proxy
time=2026-03-10T18:06:26.228+09:00 level=INFO msg="Starting llm-proxy" port=11435 upstream=http://100.94.224.71:11434
time=2026-03-10T18:06:43.946+09:00 level=INFO msg="generate request" model=qwen3.5:9b prompt_len=1880
time=2026-03-10T18:06:50.324+09:00 level=INFO msg="generate complete" model=qwen3.5:9b eval_count=27 eval_duration=825.803604ms prompt_eval_count=547
time=2026-03-10T18:06:50.324+09:00 level=INFO msg="Request completed" Method=POST Path=/api/generate StatusCode=200 Duration=6.37867875s
time=2026-03-10T18:06:52.123+09:00 level=INFO msg="generate request" model=qwen3.5:9b prompt_len=1880
time=2026-03-10T18:06:53.004+09:00 level=INFO msg="generate complete" model=qwen3.5:9b eval_count=18 eval_duration=538.063333ms prompt_eval_count=547
time=2026-03-10T18:06:53.005+09:00 level=INFO msg="Request completed" Method=POST Path=/api/generate StatusCode=200 Duration=881.410583ms
time=2026-03-10T18:06:53.945+09:00 level=INFO msg="generate request" model=qwen3.5:9b prompt_len=1880
time=2026-03-10T18:06:54.718+09:00 level=INFO msg="generate complete" model=qwen3.5:9b eval_count=15 eval_duration=445.431328ms prompt_eval_count=547
time=2026-03-10T18:06:54.718+09:00 level=INFO msg="Request completed" Method=POST Path=/api/generate StatusCode=200 Duration=773.19925ms
time=2026-03-10T18:06:55.840+09:00 level=INFO msg="generate request" model=qwen3.5:9b prompt_len=1880
time=2026-03-10T18:06:56.521+09:00 level=INFO msg="generate complete" model=qwen3.5:9b eval_count=11 eval_duration=317.866127ms prompt_eval_count=547
time=2026-03-10T18:06:56.521+09:00 level=INFO msg="Request completed" Method=POST Path=/api/generate StatusCode=200 Duration=680.951375ms

```


# Features

## Ollama
- [ ] anything more than just `/api/generate` ....

## OpenAI compatiable
- [ ] anything
