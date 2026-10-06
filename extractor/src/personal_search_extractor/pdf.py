import logging
from dataclasses import dataclass
from pathlib import Path

from pypdf import PdfReader
from pypdf.errors import (
    EmptyFileError,
    FileNotDecryptedError,
    LimitReachedError,
    PdfReadError,
    PdfStreamError,
    WrongPasswordError,
)

logging.getLogger("pypdf").setLevel(logging.ERROR)


@dataclass(frozen=True)
class PDFExtractionResult:
    text: str
    page_count: int


class PDFExtractionError(Exception):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


def extract_pdf(path: str) -> PDFExtractionResult:
    pdf_path = Path(path)

    try:
        reader = PdfReader(pdf_path)
    except FileNotFoundError as error:
        raise PDFExtractionError("file_not_found", "PDF file was not found") from error
    except PermissionError as error:
        raise PDFExtractionError("file_access", "PDF file could not be read") from error
    except (EmptyFileError, PdfReadError, PdfStreamError, LimitReachedError) as error:
        raise PDFExtractionError("invalid_pdf", "PDF file is invalid or damaged") from error
    except OSError as error:
        raise PDFExtractionError("file_access", "PDF file could not be read") from error

    if reader.is_encrypted:
        raise PDFExtractionError("encrypted_pdf", "PDF requires a password")

    page_text: list[str] = []
    try:
        for page in reader.pages:
            text = page.extract_text() or ""
            if text.strip():
                page_text.append(text.strip())
    except (FileNotDecryptedError, WrongPasswordError) as error:
        raise PDFExtractionError("encrypted_pdf", "PDF requires a password") from error
    except (PdfReadError, PdfStreamError, LimitReachedError) as error:
        raise PDFExtractionError("invalid_pdf", "PDF content could not be extracted") from error
    except OSError as error:
        raise PDFExtractionError("file_access", "PDF file could not be read") from error

    text = "\n\n".join(page_text)
    if not text:
        raise PDFExtractionError(
            "no_extractable_text",
            "PDF contains no extractable text and may require OCR",
        )

    return PDFExtractionResult(text=text, page_count=len(reader.pages))
