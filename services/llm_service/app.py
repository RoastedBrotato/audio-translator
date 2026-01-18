"""
LLM Service - Proxy to Ollama for text generation

This service provides a simplified interface to Ollama running on the host machine.
It handles prompt construction and communication with Ollama for RAG-based Q&A.

Endpoints:
- POST /generate: Generate response from LLM with context
- GET /health: Health check endpoint
"""

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from typing import Optional
import requests
import os
import logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Initialize FastAPI app
app = FastAPI(
    title="LLM Service",
    description="LLM text generation proxy to Ollama",
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

# Configuration from environment variables
OLLAMA_BASE_URL = os.getenv("OLLAMA_BASE_URL", "http://localhost:11434")
DEFAULT_MODEL = os.getenv("OLLAMA_MODEL", "llama3.1:8b")

logger.info(f"LLM Service initialized")
logger.info(f"Ollama URL: {OLLAMA_BASE_URL}")
logger.info(f"Default model: {DEFAULT_MODEL}")


# Language instructions for LLM prompts
LANGUAGE_INSTRUCTIONS = {
    "en": "Please provide a clear, concise answer in English.",
    "ar": "يرجى تقديم إجابة واضحة وموجزة باللغة العربية. (Please provide a clear, concise answer in Arabic.)",
    "ur": "براہ کرم اردو میں واضح اور جامع جواب فراہم کریں۔ (Please provide a clear, concise answer in Urdu.)",
    "hi": "कृपया हिंदी में स्पष्ट और संक्षिप्त उत्तर प्रदान करें। (Please provide a clear, concise answer in Hindi.)",
    "ml": "ദയവായി മലയാളത്തിൽ വ്യക്തവും സംക്ഷിപ്തവുമായ ഉത്തരം നൽകുക. (Please provide a clear, concise answer in Malayalam.)",
    "te": "దయచేసి తెలుగులో స్పష్టమైన మరియు సంక్షిప్త సమాధానాన్ని అందించండి. (Please provide a clear, concise answer in Telugu.)",
    "ta": "தயவுசெய்து தமிழில் தெளிவான மற்றும் சுருக்கமான பதிலை வழங்கவும். (Please provide a clear, concise answer in Tamil.)",
    "bn": "দয়া করে বাংলায় স্পষ্ট এবং সংক্ষিপ্ত উত্তর প্রদান করুন। (Please provide a clear, concise answer in Bengali.)",
    "fr": "Veuillez fournir une réponse claire et concise en français. (Please provide a clear, concise answer in French.)",
    "es": "Por favor, proporcione una respuesta clara y concisa en español. (Please provide a clear, concise answer in Spanish.)",
    "de": "Bitte geben Sie eine klare und präzise Antwort auf Deutsch. (Please provide a clear, concise answer in German.)",
    "zh": "请用中文提供清晰简洁的答案。 (Please provide a clear, concise answer in Chinese.)",
    "ja": "日本語で明確かつ簡潔な回答を提供してください。 (Please provide a clear, concise answer in Japanese.)",
    "ko": "한국어로 명확하고 간결한 답변을 제공해주세요. (Please provide a clear, concise answer in Korean.)"
}


# Request/Response models
class GenerateRequest(BaseModel):
    prompt: str
    context: str = ""
    max_tokens: Optional[int] = 500
    temperature: Optional[float] = 0.7
    language: Optional[str] = "en"

    class Config:
        json_schema_extra = {
            "example": {
                "prompt": "What were the main topics discussed?",
                "context": "[00:01:23] Alice: We need to discuss the Q4 roadmap...",
                "max_tokens": 500,
                "temperature": 0.7,
                "language": "ar"
            }
        }


class GenerateResponse(BaseModel):
    response: str
    model: str


# Endpoints
@app.post("/generate", response_model=GenerateResponse)
async def generate(request: GenerateRequest):
    """
    Generate response from LLM based on prompt and context.

    The service constructs a system prompt that instructs the LLM to answer
    questions based ONLY on the provided meeting context.

    Args:
        request: GenerateRequest with prompt, context, and generation parameters

    Returns:
        GenerateResponse with generated answer and model name
    """
    try:
        if not request.prompt or not request.prompt.strip():
            raise HTTPException(status_code=400, detail="Prompt cannot be empty")

        logger.info(f"Generating response for prompt (length: {len(request.prompt)} chars)")
        logger.info(f"Context length: {len(request.context)} chars")
        logger.info(f"Response language: {request.language}")

        # Build full prompt with system instructions and context
        language = request.language or "en"
        language_instruction = LANGUAGE_INSTRUCTIONS.get(language, LANGUAGE_INSTRUCTIONS["en"])

        full_prompt = f"""You are a helpful AI assistant answering questions about a meeting transcript.

Context from the meeting:
{request.context}

User question: {request.prompt}

{language_instruction}
- If the user asks for a summary or what was discussed, summarize the key points from the context.
- If the context is partial, answer with what is available and mention it is based on partial transcript.
- Only say "I don't have enough information" (in the target language) when the context is empty or unrelated to the question.
- Base your answer ONLY on the context provided above.
- Respond entirely in the requested language.

Answer:"""

        # Call Ollama API
        ollama_url = f"{OLLAMA_BASE_URL}/api/generate"

        payload = {
            "model": DEFAULT_MODEL,
            "prompt": full_prompt,
            "stream": False,
            "options": {
                "temperature": request.temperature,
                "num_predict": request.max_tokens
            }
        }

        logger.info(f"Calling Ollama at {ollama_url}")

        response = requests.post(
            ollama_url,
            json=payload,
            timeout=120  # 2 minute timeout for LLM generation
        )

        response.raise_for_status()
        result = response.json()

        generated_text = result.get("response", "")

        if not generated_text:
            raise HTTPException(
                status_code=500,
                detail="Ollama returned empty response"
            )

        logger.info(f"Generated response (length: {len(generated_text)} chars)")

        return GenerateResponse(
            response=generated_text,
            model=DEFAULT_MODEL
        )

    except requests.exceptions.ConnectionError:
        logger.error("Cannot connect to Ollama service")
        raise HTTPException(
            status_code=503,
            detail=f"Cannot connect to Ollama at {OLLAMA_BASE_URL}. Is Ollama running?"
        )
    except requests.exceptions.Timeout:
        logger.error("Ollama request timed out")
        raise HTTPException(
            status_code=504,
            detail="LLM generation timed out (exceeded 120 seconds)"
        )
    except requests.exceptions.HTTPError as e:
        logger.error(f"Ollama HTTP error: {str(e)}")
        raise HTTPException(
            status_code=502,
            detail=f"Ollama error: {str(e)}"
        )
    except Exception as e:
        logger.error(f"Error generating response: {str(e)}")
        raise HTTPException(
            status_code=500,
            detail=f"Generation failed: {str(e)}"
        )


@app.get("/health")
async def health_check():
    """
    Health check endpoint.

    Verifies that Ollama is accessible and returns service status.

    Returns:
        Health status with Ollama connectivity information
    """
    try:
        # Try to ping Ollama
        response = requests.get(f"{OLLAMA_BASE_URL}/api/tags", timeout=5)
        response.raise_for_status()

        models = response.json().get("models", [])
        model_names = [m.get("name") for m in models]

        # Check if our default model is available
        model_available = any(DEFAULT_MODEL in name for name in model_names)

        return {
            "status": "ok" if model_available else "degraded",
            "ollama_url": OLLAMA_BASE_URL,
            "default_model": DEFAULT_MODEL,
            "model_available": model_available,
            "available_models": model_names,
            "service": "llm-service"
        }
    except Exception as e:
        logger.warning(f"Health check failed: {str(e)}")
        return {
            "status": "degraded",
            "error": f"Cannot connect to Ollama: {str(e)}",
            "ollama_url": OLLAMA_BASE_URL,
            "service": "llm-service"
        }


# Run the application
if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        app,
        host="0.0.0.0",
        port=8007,
        log_level="info"
    )
