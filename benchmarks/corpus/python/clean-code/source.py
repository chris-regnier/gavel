import hashlib
import hmac
import os
import secrets
from typing import Optional


def hash_password(password: str) -> tuple[str, str]:
    """Hash a password with a random salt using PBKDF2."""
    salt = secrets.token_hex(32)
    key = hashlib.pbkdf2_hmac("sha256", password.encode(), salt.encode(), 100000)
    return salt, key.hex()


def verify_password(password: str, salt: str, expected_hash: str) -> bool:
    """Verify a password against a stored hash."""
    key = hashlib.pbkdf2_hmac("sha256", password.encode(), salt.encode(), 100000)
    return hmac.compare_digest(key.hex(), expected_hash)


def generate_token(length: int = 32) -> str:
    """Generate a cryptographically secure random token."""
    return secrets.token_urlsafe(length)


def get_config_value(key: str, default: Optional[str] = None) -> Optional[str]:
    """Safely retrieve a configuration value from environment."""
    return os.environ.get(key, default)
