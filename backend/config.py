from pydantic_settings import BaseSettings
from functools import lru_cache
import secrets
import os


class Settings(BaseSettings):
    #app settings
    debug: bool = False
    
    #database
    database_url: str = "postgresql+asyncpg://blog:blog@db:5432/blog"
    database_url_sync: str = "postgresql://blog:blog@db:5432/blog"
    
    #JWT
    secret_key: str = secrets.token_urlsafe(32)
    algorithm: str = "HS256"
    access_token_expire_minutes: int = 60 * 24 * 365
    
    #admin
    admin_email: str = ""
    admin_default_password: str = ""
    
    #MinIO/S3
    minio_endpoint: str = "minio:9000"
    minio_access_key: str = ""
    minio_secret_key: str = ""
    minio_bucket: str = "blog"
    minio_secure: bool = False
    
    #public URL for attachments (should be the nginx proxy URL in production)
    #In production => https://yourdomain.com
    #nginx proxy will route /blog/* to MinIO
    minio_public_url: str = ""
    
    #CORS
    cors_origins: str = "*"
    
    #avatars
    avatars_config_path: str = "./avatars/avatars.json"
    
    #uploads
    max_file_size: int = 50 * 1024 * 1024
    
    #GIF Support - Klipy API
    #get API key from: https://partner.klipy.com
    klipy_api_key: str = ""
    
    class Config:
        env_file = ".env"
        extra = "ignore"
    
    def get_attachment_base_url(self) -> str:
        """get the base URL for attachment URLs
        
        returns empty string for relative URLs (recommended for production),
        or the full public URL if explicitly configured.
        """
        if self.minio_public_url:
            return self.minio_public_url.rstrip('/')
        return ""


@lru_cache()
def get_settings():
    return Settings()