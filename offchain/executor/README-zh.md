# 链下执行器

该包实现了链工作管道，用于获取远程内容，通过句子转换器服务生成嵌入，使用本地 Ollama 模型对文档进行分类，并将记录发布到 DHT。

＃＃ 成分

- `HTTPFetcher`：使用具有可配置超时的 HTTP GET 下载任务 URL。
- `SentenceTransformerClient`：调用 Python 嵌入服务（请参阅 `offchain/services/sentence_transformer_server.py`）并返回向量嵌入。
- `OllamaZeroShotClassifier`：通过本地 Ollama 守护进程执行零样本分类。标签仅限于预先配置的列表。
- `PreprocessDocument`：剥离样板 HTML（标题、侧边栏、内容网格代码片段），提取 `<title>` 元素，并在嵌入/分类之前将主要内容区域转换为 Markdown。

## 运行嵌入服务

1. 安装依赖项：
   ```bash
   pip install sentence-transformers
   ```
2. 启动 HTTP 服务器：
   ```bash
   python offchain/services/sentence_transformer_server.py --host 0.0.0.0 --port 9000
   ```
   - 通过 `SENTENCE_TRANSFORMER_MODEL` 配置底层模型（默认：`all-MiniLM-L6-v2`）。
   - 当 GPU 可用时，将 `SENTENCE_TRANSFORMER_DEVICE` 设置为 `cuda`。

服务器公开 `POST /embed` 接受：
```json
{
  "text": "document content",
  "normalize": true,
  "model": "optional model override"
}
```
并回应：
```json
{
  "embedding": [0.1, 0.2, ...],
  "dim": 384,
  "model": "all-MiniLM-L6-v2",
  "normalize": true
}
```

使用 `GET /healthz` 进行准备情况检查。

## Ollama 零样本分类

在本地运行 Ollama 守护进程（默认端口 `11434`）并拉取所需的模型，例如：
```bash
ollama pull mistral
```

使用模型名称和标签列表配置分类器：
```go
classifier, _ := executor.NewOllamaZeroShotClassifier(executor.OllamaZeroShotConfig{
    Model:      "mistral",
    Categories: []string{"safe", "unsafe", "review"},
})
```
分类器发送确定性提示 (`temperature=0`) 并从响应中提取第一个匹配标签。如果没有返回标签，则调用失败，以便执行器可以显示错误。

## 连接执行器实例

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

执行器与具体传输保持分离，因此它可以嵌入到长期运行的工作线程中，或者针对单个任务临时触发。
