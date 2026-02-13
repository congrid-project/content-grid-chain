import os
from typing import Any, Dict, List, Optional

import chromadb
from chromadb.config import Settings
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel


CHROMA_PATH = os.environ.get("CHROMA_PATH", "./offchain/chromad/data")
CHROMA_HOST = os.environ.get("CHROMA_HOST", "127.0.0.1")
CHROMA_PORT = int(os.environ.get("CHROMA_PORT", "8000"))

app = FastAPI(title="chromad", version="0.1")

client = chromadb.PersistentClient(
    path=CHROMA_PATH,
    settings=Settings(anonymized_telemetry=False),
)


def get_collection(name: str):
    # cosine is implied by embeddings normalization, but we keep metric selection at query time minimal.
    # Chroma uses cosine by default for HNSW in many configs; we treat it as cosine.
    return client.get_or_create_collection(name=name, metadata={"hnsw:space": "cosine"})


class UpsertReq(BaseModel):
    collection: str
    id: str
    embedding: List[float]
    metadata: Optional[Dict[str, str]] = None


class DeleteReq(BaseModel):
    collection: str
    id: str


class SimilarReq(BaseModel):
    collection: str
    domain: str
    limit: int = 10


class SimilarHit(BaseModel):
    domain: str
    score: float


class SimilarResp(BaseModel):
    domain: str
    limit: int
    hits: List[SimilarHit]
    hash: str


@app.get("/healthz")
def healthz():
    return {"status": "ok"}


@app.post("/v1/upsert")
def upsert(req: UpsertReq):
    cid = req.id.strip().lower()
    if not cid:
        raise HTTPException(status_code=400, detail="id required")
    if not req.embedding:
        raise HTTPException(status_code=400, detail="embedding required")

    col = get_collection(req.collection)
    col.upsert(
        ids=[cid],
        embeddings=[req.embedding],
        metadatas=[req.metadata or {}],
    )
    return {"status": "ok"}


@app.post("/v1/delete")
def delete(req: DeleteReq):
    cid = req.id.strip().lower()
    if not cid:
        raise HTTPException(status_code=400, detail="id required")
    col = get_collection(req.collection)
    col.delete(ids=[cid])
    return {"status": "ok"}


def set_hash(domains: List[str]) -> str:
    import hashlib

    d = sorted([x.strip().lower() for x in domains if x.strip()])
    return hashlib.sha256("\n".join(d).encode("utf-8")).hexdigest()


@app.post("/v1/similar", response_model=SimilarResp)
def similar(req: SimilarReq):
    domain = req.domain.strip().lower()
    if not domain:
        raise HTTPException(status_code=400, detail="domain required")
    if req.limit <= 0:
        req.limit = 10

    col = get_collection(req.collection)

    # Fetch the domain's embedding first
    got = col.get(ids=[domain], include=["embeddings"])
    if not got or not got.get("embeddings"):
        raise HTTPException(status_code=404, detail="domain not indexed")

    emb = got["embeddings"][0]
    res = col.query(query_embeddings=[emb], n_results=req.limit + 1, include=["distances"])

    ids = res.get("ids", [[]])[0]
    dists = res.get("distances", [[]])[0]

    hits: List[SimilarHit] = []
    for i, did in enumerate(ids):
        if did == domain:
            continue
        # For cosine space, Chroma returns distance (smaller is closer). Convert to similarity-ish score.
        dist = float(dists[i]) if i < len(dists) else 0.0
        score = 1.0 - dist
        hits.append(SimilarHit(domain=did, score=score))
        if len(hits) >= req.limit:
            break

    return SimilarResp(domain=domain, limit=req.limit, hits=hits, hash=set_hash([h.domain for h in hits]))


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host=CHROMA_HOST, port=CHROMA_PORT)
