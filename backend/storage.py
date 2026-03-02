from minio import Minio
from minio.error import S3Error
from config import get_settings
import io
import uuid
import logging

logger = logging.getLogger(__name__)
settings = get_settings()

minio_client = Minio(
    settings.minio_endpoint,
    access_key=settings.minio_access_key,
    secret_key=settings.minio_secret_key,
    secure=settings.minio_secure
)


async def init_minio():
    try:
        if not minio_client.bucket_exists(settings.minio_bucket):
            minio_client.make_bucket(settings.minio_bucket)

            policy = {
                "Version": "2012-10-17",
                "Statement": [
                    {
                        "Effect": "Allow",
                        "Principal": {"AWS": "*"},
                        "Action": ["s3:GetObject"],
                        "Resource": [f"arn:aws:s3:::{settings.minio_bucket}/*"]
                    }
                ]
            }
            import json
            minio_client.set_bucket_policy(settings.minio_bucket, json.dumps(policy))
            logger.info(f"Created MinIO bucket: {settings.minio_bucket}")
        else:
            logger.info(f"MinIO bucket already exists: {settings.minio_bucket}")
    except S3Error as e:
        logger.error(f"MinIO initialization error: {e}")
        raise


def upload_file(file_data: bytes, filename: str, content_type: str, folder: str = "uploads") -> str:
    """upload a file to MinIO and return the object path
    
    args:
        file_data: the file content as bytes
        filename: original filename
        content_type: MIME type of the file
        folder: folder/prefix in the bucket
        
    returns:
        the object name (path within the bucket)
    """
    ext = filename.rsplit('.', 1)[-1].lower() if '.' in filename else ''
    unique_name = f"{uuid.uuid4()}.{ext}" if ext else str(uuid.uuid4())
    object_name = f"{folder}/{unique_name}"
    
    try:
        minio_client.put_object(
            settings.minio_bucket,
            object_name,
            io.BytesIO(file_data),
            length=len(file_data),
            content_type=content_type
        )
        logger.info(f"Uploaded file: {object_name} ({len(file_data)} bytes)")
        return object_name
    except S3Error as e:
        logger.error(f"Failed to upload file {filename}: {e}")
        raise


def get_file_url(object_name: str) -> str:
    """get the URL for accessing a file
    
    args:
        object_name: the object path within the bucket
        
    returns:
        URL to access the file (relative URL through nginx proxy)
    """
    base_url = settings.get_attachment_base_url()
    
    if base_url:
        return f"{base_url}/{settings.minio_bucket}/{object_name}"
    else:
        return f"/{settings.minio_bucket}/{object_name}"


def delete_file(object_name: str) -> bool:
    """delete a file from MinIO
    
    args:
        object_name: the object path within the bucket
        
    returns:
        true if successful => false otherwise
    """
    try:
        minio_client.remove_object(settings.minio_bucket, object_name)
        logger.info(f"Deleted file: {object_name}")
        return True
    except S3Error as e:
        logger.error(f"Error deleting file {object_name}: {e}")
        return False


def get_presigned_url(object_name: str, expires_hours: int = 24) -> str:
    """get a presigned URL for temporary direct access
    
    args:
        object_name: the object path within the bucket
        expires_hours: how long the URL should be valid
        
    returns:
        presigned URL for direct access
    """
    from datetime import timedelta
    try:
        url = minio_client.presigned_get_object(
            settings.minio_bucket,
            object_name,
            expires=timedelta(hours=expires_hours)
        )
        return url
    except S3Error as e:
        logger.error(f"Error generating presigned URL for {object_name}: {e}")
        raise


def file_exists(object_name: str) -> bool:
    """vheck if a file exists in MinIO
    
    args:
        object_name: the object path within the bucket
        
    returns:
        true if the file exists => false otherwise
    """
    try:
        minio_client.stat_object(settings.minio_bucket, object_name)
        return True
    except S3Error:
        return False