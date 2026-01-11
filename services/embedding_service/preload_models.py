"""
Preload embedding models during Docker build.
This ensures the model is cached and ready when the container starts.
"""
from sentence_transformers import SentenceTransformer

print("Downloading sentence-transformers/all-MiniLM-L6-v2...")
model = SentenceTransformer('sentence-transformers/all-MiniLM-L6-v2')
print(f"Model downloaded successfully. Embedding dimension: {model.get_sentence_embedding_dimension()}")
