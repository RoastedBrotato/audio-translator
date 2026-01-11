"""
Embedding Service - Generate text embeddings using sentence-transformers

This service provides endpoints for generating embeddings from text using
the all-MiniLM-L6-v2 model (384 dimensions, fast and lightweight).

Endpoints:
- POST /embed: Generate embedding for a single text
- POST /embed-batch: Generate embeddings for multiple texts (more efficient)
- GET /health: Health check endpoint
"""

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer
from typing import List
import uvicorn
import logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Initialize FastAPI app
app = FastAPI(
    title="Embedding Service",
    description="Text embedding generation using sentence-transformers",
    version="1.0.0"
)

# Add CORS middleware for Go backend communication
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Load embedding model on startup
MODEL_NAME = 'sentence-transformers/all-MiniLM-L6-v2'
logger.info(f"Loading embedding model: {MODEL_NAME}")
model = SentenceTransformer(MODEL_NAME)
EMBEDDING_DIM = model.get_sentence_embedding_dimension()
logger.info(f"Model loaded successfully. Embedding dimension: {EMBEDDING_DIM}")


# Request/Response models
class EmbedRequest(BaseModel):
    text: str

    class Config:
        json_schema_extra = {
            "example": {
                "text": "What were the main topics discussed in the meeting?"
            }
        }


class EmbedResponse(BaseModel):
    embedding: List[float]
    dimension: int


class EmbedBatchRequest(BaseModel):
    texts: List[str]

    class Config:
        json_schema_extra = {
            "example": {
                "texts": [
                    "First chunk of meeting transcript",
                    "Second chunk of meeting transcript"
                ]
            }
        }


class EmbedBatchResponse(BaseModel):
    embeddings: List[List[float]]
    dimension: int
    count: int


# Endpoints
@app.post("/embed", response_model=EmbedResponse)
async def embed_text(request: EmbedRequest):
    """
    Generate embedding for a single text.

    Args:
        request: EmbedRequest containing text to embed

    Returns:
        EmbedResponse with embedding vector and dimension
    """
    try:
        if not request.text or not request.text.strip():
            raise HTTPException(status_code=400, detail="Text cannot be empty")

        logger.info(f"Generating embedding for text (length: {len(request.text)} chars)")

        # Generate embedding
        embedding = model.encode(request.text, convert_to_numpy=True)

        return EmbedResponse(
            embedding=embedding.tolist(),
            dimension=len(embedding)
        )

    except Exception as e:
        logger.error(f"Error generating embedding: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Embedding generation failed: {str(e)}")


@app.post("/embed-batch", response_model=EmbedBatchResponse)
async def embed_batch(request: EmbedBatchRequest):
    """
    Generate embeddings for multiple texts (batch mode - more efficient).

    Args:
        request: EmbedBatchRequest containing list of texts

    Returns:
        EmbedBatchResponse with list of embedding vectors
    """
    try:
        if not request.texts:
            raise HTTPException(status_code=400, detail="Texts list cannot be empty")

        # Filter out empty texts
        valid_texts = [text.strip() for text in request.texts if text.strip()]

        if not valid_texts:
            raise HTTPException(status_code=400, detail="All texts are empty")

        logger.info(f"Generating embeddings for {len(valid_texts)} texts (batch mode)")

        # Generate embeddings in batch (more efficient)
        embeddings = model.encode(
            valid_texts,
            convert_to_numpy=True,
            batch_size=32,
            show_progress_bar=False
        )

        return EmbedBatchResponse(
            embeddings=embeddings.tolist(),
            dimension=EMBEDDING_DIM,
            count=len(embeddings)
        )

    except Exception as e:
        logger.error(f"Error generating batch embeddings: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Batch embedding generation failed: {str(e)}")


@app.get("/health")
async def health_check():
    """
    Health check endpoint.

    Returns:
        Health status with model information
    """
    return {
        "status": "ok",
        "model": MODEL_NAME,
        "dimension": EMBEDDING_DIM,
        "service": "embedding-service"
    }


# Run the application
if __name__ == "__main__":
    uvicorn.run(
        app,
        host="0.0.0.0",
        port=8006,
        log_level="info"
    )
