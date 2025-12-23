from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from deep_translator import GoogleTranslator

app = FastAPI()

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

class TranslateRequest(BaseModel):
    text: str
    source_lang: str = "en"
    target_lang: str = "ar"

@app.post("/translate")
async def translate(req: TranslateRequest):
    """Translate text from source language to target language"""
    try:
        if not req.text or not req.text.strip():
            return {"translation": ""}
        
        # Map language codes
        # deep_translator uses full language names for some languages
        lang_map = {
            "ar": "arabic",
            "ur": "urdu",
            "en": "english",
            "auto": "auto"
        }
        
        source = lang_map.get(req.source_lang, req.source_lang)
        target = lang_map.get(req.target_lang, req.target_lang)
        
        translator = GoogleTranslator(source=source, target=target)
        translation = translator.translate(req.text)
        
        return {"translation": translation}
    
    except Exception as e:
        print(f"Translation error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/health")
async def health():
    return {"status": "ok"}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="127.0.0.1", port=8004)
