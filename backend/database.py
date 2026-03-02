from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession, async_sessionmaker
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker
from config import get_settings
from models import Base, User
import bcrypt
import logging

logger = logging.getLogger(__name__)
settings = get_settings()


def hash_password(password: str) -> str:
    return bcrypt.hashpw(
        password.encode('utf-8'), 
        bcrypt.gensalt()
    ).decode('utf-8')


async_engine = create_async_engine(
    settings.database_url,
    echo=settings.debug,
    pool_pre_ping=True,
    pool_size=10,
    max_overflow=20,
)

AsyncSessionLocal = async_sessionmaker(
    bind=async_engine,
    class_=AsyncSession,
    expire_on_commit=False,
)

sync_engine = create_engine(
    settings.database_url_sync, 
    echo=settings.debug,
    pool_pre_ping=True,
)
SyncSessionLocal = sessionmaker(bind=sync_engine)


async def get_db():
    async with AsyncSessionLocal() as session:
        try:
            yield session
        finally:
            await session.close()


async def init_db():
    logger.info("Initializing database...")
    
    async with async_engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    
    async with AsyncSessionLocal() as session:
        from sqlalchemy import select
        result = await session.execute(
            select(User).where(User.email == settings.admin_email)
        )
        admin = result.scalar_one_or_none()
        
        if not admin:
            admin = User(
                email=settings.admin_email,
                username="admin",
                password_hash=hash_password(settings.admin_default_password),
                is_admin=True,
                must_change_password=True,
                avatar="default"
            )
            session.add(admin)
            await session.commit()
            logger.info(f"Created admin user: {settings.admin_email}")
        else:
            logger.info(f"Admin user already exists: {settings.admin_email}")