# Off-chain Executor

This package implements the chain worker pipeline that fetches remote content, produces embeddings via a sentence-transformer service, classifies the document with a local Ollama model, and publishes records to the DHT.

## Components

- `HTTPFetcher`: downloads task URLs using HTTP GET with configurable timeouts.
- `SentenceTransformerClient`: calls the Python embedding service (see `offchain/services/sentence_transformer_server.py`) and returns vector embeddings.
- `OllamaZeroShotClassifier`: performs zero-shot classification through a local Ollama daemon. Labels are constrained to a pre-configured list.
- `PreprocessDocument`: strips boilerplate HTML (headers, sidebars, Content Grid code snippets), extracts the `<title>` element, and converts the main content area into Markdown before embedding/classification.

## Running the Embedding Service

1. Install dependencies:
   ```bash
   pip install sentence-transformers
   ```
2. Launch the HTTP server:
   ```bash
   python offchain/services/sentence_transformer_server.py --host 0.0.0.0 --port 9000
   ```
   - Configure the underlying model via `SENTENCE_TRANSFORMER_MODEL` (default: `all-MiniLM-L6-v2`).
   - Set `SENTENCE_TRANSFORMER_DEVICE` to `cuda` when a GPU is available.

The server exposes `POST /embed` which accepts:
```json
{
  "text": "document content",
  "normalize": true,
  "model": "optional model override"
}
```
and responds with:
```json
{
  "embedding": [0.1, 0.2, ...],
  "dim": 384,
  "model": "all-MiniLM-L6-v2",
  "normalize": true
}
```

Use `GET /healthz` for readiness checks.

## Ollama Zero-shot Classification

Run an Ollama daemon locally (default port `11434`) and pull the desired model, for example:
```bash
ollama pull mistral
```

Configure the classifier with the model name and label list:
```go
classifier, _ := executor.NewOllamaZeroShotClassifier(executor.OllamaZeroShotConfig{
    Model:      "mistral",
    Categories: []string{"safe", "unsafe", "review"},
})
```
The classifier sends a deterministic prompt (`temperature=0`) and extracts the first matching label from the response. If no label is returned, the call fails so the executor can surface the error.

## Wiring an Executor Instance

```go
store := dhtService // implement dht.VectorStore
fetcher := executor.NewHTTPFetcher(10 * time.Second)
embedder, _ := executor.NewSentenceTransformerClient(executor.DefaultSentenceTransformerConfig())
classifier, _ := executor.NewOllamaZeroShotClassifier(executor.OllamaZeroShotConfig{
    Model:      "mistral",
    Categories: []string{"safe", "unsafe"},
})
exec, _ := executor.NewExecutor(executor.DefaultConfig(), store, fetcher, embedder, classifier)
```

The executor remains decoupled from concrete transports so it can be embedded in long-running workers or triggered ad-hoc for individual tasks.
