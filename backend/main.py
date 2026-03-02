from fastapi import FastAPI, Depends, HTTPException, status, WebSocket, WebSocketDisconnect, UploadFile, File, Form, Query
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles
from fastapi.responses import JSONResponse
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, delete
from sqlalchemy.orm import selectinload
from typing import Optional, List, cast
from contextlib import asynccontextmanager
from datetime import datetime, timezone
import json
import os
import logging
import httpx

from config import get_settings
from database import get_db, init_db
from models import User, Message, Reaction, CustomEmoji
from schemas import (
    LoginRequest, PasswordChangeRequest, TokenResponse,
    GuestCreate, UserResponse, MessageResponse,
    MessageList, ReactionCreate, Attachment, MessageReplyInfo,
    CommandResponse, CustomEmojiResponse, GifSearchResult, GifSearchResponse
)
from auth import (
    verify_password, hash_password, create_access_token,
    get_current_user, get_current_admin, generate_guest_id
)
from websocket_manager import manager
from storage import init_minio, upload_file, get_file_url, delete_file

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

settings = get_settings()


@asynccontextmanager
async def lifespan(app: FastAPI):
    logger.info("Starting up application...")
    await init_db()
    await init_minio()
    os.makedirs(os.path.dirname(settings.avatars_config_path), exist_ok=True)
    os.makedirs("./avatars", exist_ok=True)
    logger.info("Application startup complete")
    yield
    logger.info("Shutting down application...")


docs_url = "/docs" if settings.debug else None
redoc_url = "/redoc" if settings.debug else None

app = FastAPI(
    title="Blog API",
    description="Real-time chat-style blog API",
    version="1.0.0",
    lifespan=lifespan,
    docs_url=docs_url,
    redoc_url=redoc_url
)

cors_origins = settings.cors_origins.split(",") if settings.cors_origins != "*" else ["*"]

app.add_middleware(
    CORSMiddleware,
    allow_origins=cors_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.mount("/avatars", StaticFiles(directory="avatars"), name="avatars")


# ============ EXCEPTION HANDLERS ============

@app.exception_handler(Exception)
async def global_exception_handler(request, exc):
    logger.error(f"Unhandled exception: {exc}", exc_info=True)
    return JSONResponse(
        status_code=500,
        content={"detail": "Internal server error"}
    )


# ============ AUTH ROUTES ============

@app.post("/api/auth/login", response_model=TokenResponse)
async def login(request: LoginRequest, db: AsyncSession = Depends(get_db)):
    result = await db.execute(
        select(User).where(User.email == request.email, User.is_admin == True)
    )
    user = result.scalar_one_or_none()
    
    if not user or not user.password_hash or not verify_password(request.password, user.password_hash):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Invalid credentials")
    
    access_token = create_access_token(data={"sub": str(user.id)})
    return TokenResponse(access_token=access_token, must_change_password=bool(user.must_change_password))


@app.post("/api/auth/change-password")
async def change_password(
    request: PasswordChangeRequest,
    user: User = Depends(get_current_admin),
    db: AsyncSession = Depends(get_db)
):
    if len(request.new_password) < 8:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="Password must be at least 8 characters")
    
    user.password_hash = hash_password(request.new_password)
    user.must_change_password = False
    await db.commit()
    return {"message": "Password changed successfully"}


@app.post("/api/auth/guest", response_model=TokenResponse)
async def create_guest(request: GuestCreate = GuestCreate(), db: AsyncSession = Depends(get_db)):
    guest_id = generate_guest_id()
    guest = User(username=guest_id, is_admin=False, avatar=request.avatar)
    db.add(guest)
    await db.commit()
    await db.refresh(guest)
    
    access_token = create_access_token(data={"sub": str(guest.id)})
    return TokenResponse(access_token=access_token)


@app.get("/api/auth/me", response_model=UserResponse)
async def get_me(user: User = Depends(get_current_user)):
    if not user:
        raise HTTPException(status_code=401, detail="Not authenticated")
    return UserResponse(
        id=user.id,
        username=user.username,
        is_admin=user.is_admin,
        can_post=user.can_post,
        avatar=user.avatar or "default",
        created_at=user.created_at
    )


@app.patch("/api/auth/avatar")
async def update_avatar(avatar: str, user: User = Depends(get_current_user), db: AsyncSession = Depends(get_db)):
    if not user:
        raise HTTPException(status_code=401, detail="Not authenticated")
    user.avatar = avatar
    await db.commit()
    return {"message": "Avatar updated", "avatar": avatar}


# ============ GIF ROUTES ============

@app.get("/api/gifs/search", response_model=GifSearchResponse)
async def search_gifs(
    q: str = Query(..., min_length=1, description="Search query"),
    limit: int = Query(20, ge=1, le=50),
    pos: Optional[str] = Query(None, description="Pagination position"),
    user: User = Depends(get_current_user)
):
    """search for GIFs using Klipy API"""
    if not settings.klipy_api_key:
        raise HTTPException(
            status_code=503, 
            detail="GIF search not configured. Please set KLIPY_API_KEY."
        )
    
    async with httpx.AsyncClient() as client:
        params = {
            "key": settings.klipy_api_key,
            "q": q,
            "limit": limit,
            "media_filter": "gif,tinygif",
            "contentfilter": "medium"
        }
        if pos:
            params["pos"] = pos
        
        try:
            response = await client.get(
                "https://api.klipy.com/v2/search",
                params=params,
                timeout=10.0
            )
            response.raise_for_status()
            data = response.json()
        except httpx.HTTPError as e:
            logger.error(f"Klipy API error: {e}")
            raise HTTPException(status_code=503, detail="GIF search temporarily unavailable")
    
    results = []
    for item in data.get("results", []):
        media_formats = item.get("media_formats", {})
        gif = media_formats.get("gif", {})
        tinygif = media_formats.get("tinygif", {})
        
        if gif.get("url"):
            results.append(GifSearchResult(
                id=item.get("id", ""),
                title=item.get("title", ""),
                url=gif.get("url", ""),
                preview_url=tinygif.get("url", gif.get("url", "")),
                width=gif.get("dims", [0, 0])[0],
                height=gif.get("dims", [0, 0])[1]
            ))
    
    return GifSearchResponse(
        results=results,
        next=data.get("next")
    )


@app.get("/api/gifs/trending", response_model=GifSearchResponse)
async def trending_gifs(
    limit: int = Query(20, ge=1, le=50),
    pos: Optional[str] = Query(None),
    user: User = Depends(get_current_user)
):
    """get trending GIFs"""
    if not settings.klipy_api_key:
        raise HTTPException(status_code=503, detail="GIF search not configured")
    
    async with httpx.AsyncClient() as client:
        params = {
            "key": settings.klipy_api_key,
            "limit": limit,
            "media_filter": "gif,tinygif",
            "contentfilter": "medium"
        }
        if pos:
            params["pos"] = pos
        
        try:
            response = await client.get(
                "https://api.klipy.com/v2/featured",
                params=params,
                timeout=10.0
            )
            response.raise_for_status()
            data = response.json()
        except httpx.HTTPError as e:
            logger.error(f"Klipy API error: {e}")
            raise HTTPException(status_code=503, detail="GIF search temporarily unavailable")
    
    results = []
    for item in data.get("results", []):
        media_formats = item.get("media_formats", {})
        gif = media_formats.get("gif", {})
        tinygif = media_formats.get("tinygif", {})
        
        if gif.get("url"):
            results.append(GifSearchResult(
                id=item.get("id", ""),
                title=item.get("title", ""),
                url=gif.get("url", ""),
                preview_url=tinygif.get("url", gif.get("url", "")),
                width=gif.get("dims", [0, 0])[0],
                height=gif.get("dims", [0, 0])[1]
            ))
    
    return GifSearchResponse(results=results, next=data.get("next"))


# ============ CUSTOM EMOJI ROUTES ============

@app.get("/api/emojis")
async def list_custom_emojis(db: AsyncSession = Depends(get_db)):
    """list all custom emojis"""
    result = await db.execute(
        select(CustomEmoji).order_by(CustomEmoji.name)
    )
    emojis = result.scalars().all()
    return {"emojis": [CustomEmojiResponse.model_validate(e) for e in emojis]}


@app.post("/api/emojis", response_model=CustomEmojiResponse)
async def create_custom_emoji(
    name: str = Form(...),
    file: UploadFile = File(...),
    user: User = Depends(get_current_admin),
    db: AsyncSession = Depends(get_db)
):
    #validate name
    import re
    if not re.match(r'^[a-zA-Z0-9_]{2,50}$', name):
        raise HTTPException(
            status_code=400, 
            detail="Emoji name must be 2-50 alphanumeric characters or underscores"
        )
    
    #check if name exists
    existing = await db.execute(select(CustomEmoji).where(CustomEmoji.name == name))
    if existing.scalar_one_or_none():
        raise HTTPException(status_code=400, detail="Emoji name already exists")
    
    #validate file
    if not file.content_type or not file.content_type.startswith("image/"):
        raise HTTPException(status_code=400, detail="File must be an image")
    
    file_data = await file.read()
    if len(file_data) > settings.max_file_size:
        raise HTTPException(
            status_code=400,
            detail="File exceeds maximum allowed size (5mb)"
        )
    
    #upload to storage
    object_name = upload_file(file_data, f"{name}.png", file.content_type, "emojis")
    file_url = get_file_url(object_name)
    
    emoji = CustomEmoji(
        name=name,
        url=file_url,
        object_name=object_name,
        created_by_id=user.id
    )
    db.add(emoji)
    await db.commit()
    await db.refresh(emoji)
    
    logger.info(f"Created custom emoji :{name}: by {user.username}")
    
    #broadcast new emoji to all clients
    await manager.broadcast({
        "type": "custom_emoji_added",
        "data": {"id": emoji.id, "name": emoji.name, "url": emoji.url}
    })
    
    return emoji


@app.delete("/api/emojis/{emoji_id}")
async def delete_custom_emoji(
    emoji_id: str,
    user: User = Depends(get_current_admin),
    db: AsyncSession = Depends(get_db)
):
    """delete a custom emoji (admin only)"""
    result = await db.execute(select(CustomEmoji).where(CustomEmoji.id == emoji_id))
    emoji = result.scalar_one_or_none()
    
    if not emoji:
        raise HTTPException(status_code=404, detail="Emoji not found")
    
    #delete from storage
    if emoji.object_name:
        try:
            delete_file(emoji.object_name)
        except Exception as e:
            logger.warning(f"Failed to delete emoji file: {e}")
    
    await db.delete(emoji)
    await db.commit()
    
    logger.info(f"Deleted custom emoji :{emoji.name}:")
    
    #broadcast emoji removal
    await manager.broadcast({
        "type": "custom_emoji_removed",
        "data": {"id": emoji_id, "name": emoji.name}
    })
    
    return {"message": "Emoji deleted"}


# ============ MESSAGE ROUTES ============

def build_message_response(message: Message) -> MessageResponse:
    reaction_map = {}

    reactions = getattr(message, 'reactions', None) or []
    for reaction in reactions:
        if reaction.emoji not in reaction_map:
            reaction_map[reaction.emoji] = {
                "emoji": reaction.emoji, 
                "count": 0, 
                "users": [],
                "user_avatars": [],
                #add custom emoji URL if present
                "custom_emoji_url": reaction.custom_emoji.url if reaction.custom_emoji else None
            }
        reaction_map[reaction.emoji]["count"] += 1
        if reaction.user:
            reaction_map[reaction.emoji]["users"].append(reaction.user.username)
            reaction_map[reaction.emoji]["user_avatars"].append(reaction.user.avatar or "default")
    
    attachments = [Attachment(**a) for a in (message.attachments or [])]
    
    reply_info = None
    if message.parent:
        reply_info = MessageReplyInfo(
            id=str(message.parent.id),
            content=message.parent.content or "Attachment",
            author_username=message.parent.author.username if message.parent.author else "Unknown"
        )
    
    return MessageResponse(
        id=str(message.id),
        content=message.content,
        author_id=str(message.author_id),
        author_username=message.author.username if message.author else "Unknown",
        author_avatar=message.author.avatar if message.author else "default",
        is_admin=message.author.is_admin if message.author else False,
        is_pinned=message.is_pinned if message.is_pinned is not None else False,
        attachments=attachments,
        reactions=list(reaction_map.values()),
        created_at=message.created_at or datetime.now(timezone.utc),
        updated_at=message.updated_at,
        reply_to=reply_info
    )


@app.get("/api/messages", response_model=MessageList)
async def get_messages(limit: int = 50, before: Optional[str] = None, db: AsyncSession = Depends(get_db)):
    limit = min(max(1, limit), 100)
    
    pinned_messages_list = []
    if not before:
        pinned_query = select(Message).options(
            selectinload(Message.author),
            selectinload(Message.reactions).selectinload(Reaction.user),
            selectinload(Message.reactions).selectinload(Reaction.custom_emoji), # Load custom emoji
            selectinload(Message.parent).selectinload(Message.author)
        ).where(Message.is_pinned == True).order_by(Message.created_at.desc())
        
        pinned_result = await db.execute(pinned_query)
        pinned_objs = pinned_result.scalars().all()
        pinned_messages_list = [build_message_response(m) for m in pinned_objs]

    query = select(Message).options(
        selectinload(Message.author),
        selectinload(Message.reactions).selectinload(Reaction.user),
        selectinload(Message.reactions).selectinload(Reaction.custom_emoji), # Load custom emoji
        selectinload(Message.parent).selectinload(Message.author)
    ).order_by(Message.created_at.desc()).limit(limit + 1)
    
    if before:
        before_msg = await db.execute(select(Message).where(Message.id == before))
        before_msg = before_msg.scalar_one_or_none()
        if before_msg:
            query = query.where(Message.created_at < before_msg.created_at)
    
    result = await db.execute(query)
    messages = result.scalars().all()
    
    has_more = len(messages) > limit
    if has_more:
        messages = messages[:-1]
    
    messages = list(reversed(messages))
    
    return MessageList(
        messages=[build_message_response(m) for m in messages],
        pinned_messages=pinned_messages_list,
        total=len(messages),
        has_more=has_more
    )


async def handle_slash_command(
    command: str, 
    args: str, 
    user: User, 
    db: AsyncSession
) -> Optional[dict]:

    if command == "/clear":
        await db.execute(delete(Reaction))
        await db.execute(delete(Message))
        await db.commit()
        
        await manager.broadcast({"type": "chat_cleared", "data": {}})
        logger.info(f"Chat cleared by admin {user.username}")
        
        return {
            "success": True,
            "command": "clear",
            "message": "Chat cleared successfully"
        }

    elif command == "/pin":
        if not args:
            raise HTTPException(
                status_code=400, 
                detail="Usage: /pin <message content> - Creates a pinned message"
            )
        return None
    
    return None


@app.post("/api/messages")
async def create_message(
    content: Optional[str] = Form(None),
    reply_to_id: Optional[str] = Form(None),
    gif_url: Optional[str] = Form(None),  #for GIF messages
    gif_id: Optional[str] = Form(None),
    gif_preview_url: Optional[str] = Form(None),
    files: List[UploadFile] = File(default=[]),
    user: User = Depends(get_current_admin),
    db: AsyncSession = Depends(get_db)
):
    is_pinned_init = False
    
    # === SLASH COMMANDS ===
    if content and content.startswith("/"):
        command_parts = content.strip().split(" ", 1)
        command = command_parts[0].lower()
        args = command_parts[1] if len(command_parts) > 1 else ""

        result = await handle_slash_command(command, args, user, db)
        
        if result is not None:
            return JSONResponse(content=result)
        
        if command == "/pin" and args:
            content = args
            is_pinned_init = True

    if not content and not files and not gif_url:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST, 
            detail="Message must have content, attachments, or a GIF"
        )
    
    attachments = []
    
    #handle GIF attachment
    if gif_url:
        attachments.append({
            "type": "gif",
            "url": gif_url,
            "name": "GIF",
            "gif_id": gif_id,
            "preview_url": gif_preview_url
        })
    
    for file in files:
        if file.filename:
            content_type = file.content_type or "application/octet-stream"
            if content_type.startswith("image/"):
                file_type, folder = "image", "images"
            elif content_type.startswith("audio/"):
                file_type, folder = "audio", "audio"
            elif content_type.startswith("video/"):
                file_type, folder = "video", "video"
            else:
                file_type, folder = "file", "files"
            
            file_data = await file.read()
            
            #check file size
            if len(file_data) > settings.max_file_size:
                raise HTTPException(
                    status_code=400,
                    detail=f"File {file.filename} exceeds maximum size of {settings.max_file_size // (1024*1024)}MB"
                )
            
            object_name = upload_file(file_data, file.filename, content_type, folder)
            file_url = get_file_url(object_name)
            
            attachments.append({
                "type": file_type,
                "url": file_url,
                "name": file.filename,
                "size": len(file_data),
                "object_name": object_name
            })
    
    message = Message(
        content=content, 
        author_id=user.id, 
        attachments=attachments,
        reply_to_id=reply_to_id,
        is_pinned=is_pinned_init
    )
    db.add(message)
    await db.commit()
    
    result = await db.execute(
        select(Message).options(
            selectinload(Message.author),
            selectinload(Message.reactions).selectinload(Reaction.user),
            selectinload(Message.reactions).selectinload(Reaction.custom_emoji),
            selectinload(Message.parent).selectinload(Message.author)
        ).where(Message.id == message.id)
    )
    message = result.scalar_one()
    
    response = build_message_response(message)
    await manager.broadcast({"type": "new_message", "data": response.model_dump(mode="json")})
    
    return response


@app.post("/api/messages/{message_id}/pin")
async def toggle_pin_message(
    message_id: str, 
    user: User = Depends(get_current_admin), 
    db: AsyncSession = Depends(get_db)
):
    """toggle pin status for a message"""
    result = await db.execute(select(Message).where(Message.id == message_id))
    message = result.scalar_one_or_none()
    
    if not message:
        raise HTTPException(status_code=404, detail="Message not found")
    
    message.is_pinned = not message.is_pinned
    await db.commit()
    
    action = "pinned" if message.is_pinned else "unpinned"
    logger.info(f"Message {message_id} {action} by {user.username}")
    
    await manager.broadcast({
        "type": "message_pinned_update",
        "data": {
            "message_id": message_id,
            "is_pinned": message.is_pinned
        }
    })
    
    return {"message": f"Message {action}", "is_pinned": message.is_pinned}


@app.delete("/api/messages/{message_id}")
async def delete_message(
    message_id: str, 
    user: User = Depends(get_current_admin), 
    db: AsyncSession = Depends(get_db)
):
    result = await db.execute(select(Message).where(Message.id == message_id))
    message = result.scalar_one_or_none()
    
    if not message:
        raise HTTPException(status_code=404, detail="Message not found")

    if message.attachments:
        for attachment in message.attachments:
            if isinstance(attachment, dict) and "object_name" in attachment:
                obj_name = attachment.get("object_name")
                if isinstance(obj_name, str):
                    try:
                        delete_file(obj_name)
                    except Exception as e:
                        logger.warning(f"Failed to delete attachment {obj_name}: {e}")
    
    await db.delete(message)
    await db.commit()
    
    logger.info(f"Message {message_id} deleted by {user.username}")
    
    await manager.broadcast({"type": "message_deleted", "data": {"message_id": message_id}})
    return {"message": "Message deleted"}


# ============ REACTION ROUTES ============

@app.post("/api/messages/{message_id}/reactions")
async def add_reaction(
    message_id: str,
    request: ReactionCreate,
    user: User = Depends(get_current_user),
    db: AsyncSession = Depends(get_db)
):
    if not user:
        raise HTTPException(status_code=401, detail="Not authenticated")
    
    if not request.emoji or len(request.emoji) > 50:
        raise HTTPException(status_code=400, detail="Invalid emoji")
    
    result = await db.execute(select(Message).where(Message.id == message_id))
    message = result.scalar_one_or_none()
    if not message:
        raise HTTPException(status_code=404, detail="Message not found")
    
    #check for custom emoji
    custom_emoji = None
    custom_emoji_url = None
    if request.custom_emoji_id:
        result = await db.execute(select(CustomEmoji).where(CustomEmoji.id == request.custom_emoji_id))
        custom_emoji = result.scalar_one_or_none()
        if custom_emoji:
            custom_emoji_url = custom_emoji.url
    
    result = await db.execute(
        select(Reaction).where(
            Reaction.message_id == message_id,
            Reaction.user_id == user.id,
            Reaction.emoji == request.emoji
        )
    )
    existing = result.scalar_one_or_none()
    
    if existing:
        await db.delete(existing)
        await db.commit()
        await manager.broadcast({
            "type": "reaction_removed",
            "data": {
                "message_id": message_id, 
                "emoji": request.emoji, 
                "user_id": str(user.id), 
                "username": user.username,
                "avatar": user.avatar or "default",
                "custom_emoji_url": custom_emoji_url
            }
        })
        return {"message": "Reaction removed", "action": "removed"}
    
    reaction = Reaction(
        message_id=message_id, 
        user_id=user.id, 
        emoji=request.emoji,
        custom_emoji_id=request.custom_emoji_id
    )
    db.add(reaction)
    await db.commit()
    
    await manager.broadcast({
        "type": "reaction_added",
        "data": {
            "message_id": message_id, 
            "emoji": request.emoji, 
            "user_id": str(user.id), 
            "username": user.username,
            "avatar": user.avatar or "default",
            "custom_emoji_url": custom_emoji_url
        }
    })
    return {"message": "Reaction added", "action": "added"}


# ============ WEBSOCKET ============

@app.websocket("/ws")
async def websocket_endpoint(
    websocket: WebSocket, 
    token: Optional[str] = None, 
    db: AsyncSession = Depends(get_db)
):
    """WebSocket endpoint for real-time updates"""
    user = None
    
    if token:
        from jose import jwt, JWTError
        try:
            payload = jwt.decode(token, settings.secret_key, algorithms=[settings.algorithm])
            user_id = cast(Optional[str], payload.get("sub"))
            if user_id:
                result = await db.execute(select(User).where(User.id == user_id))
                user = result.scalar_one_or_none()
        except JWTError:
            logger.warning("Invalid JWT token in WebSocket connection")
    
    if not user:
        guest_id = generate_guest_id()
        user = User(username=guest_id, is_admin=False, avatar="default")
        db.add(user)
        await db.commit()
        await db.refresh(user)
    
    user_info = {
        "username": user.username, 
        "avatar": user.avatar or "default", 
        "is_admin": user.is_admin
    }
    await manager.connect(websocket, str(user.id), user_info)
    
    new_token = create_access_token(data={"sub": str(user.id)}) if not token else None
    
    try:
        await websocket.send_json({
            "type": "connected",
            "data": {
                "user_id": str(user.id),
                "username": user.username,
                "avatar": user.avatar or "default",
                "is_admin": user.is_admin,
                "can_post": user.can_post,
                "token": new_token,
                "online_users": manager.get_online_users(),
                "online_count": manager.get_online_count()
            }
        })

        while True:
            data = await websocket.receive_json()
            
            if data.get("type") == "typing":
                await manager.broadcast({
                    "type": "typing",
                    "data": {"user_id": str(user.id), "username": user.username}
                })
            
            elif data.get("type") == "update_avatar":
                new_avatar = data.get("avatar", "default")
                user.avatar = new_avatar
                await db.commit()
                await manager.broadcast({
                    "type": "user_avatar_changed",
                    "data": {"user_id": str(user.id), "avatar": new_avatar}
                })
    
    except WebSocketDisconnect:
        await manager.disconnect(str(user.id))
    except Exception as e:
        logger.error(f"WebSocket error: {e}")
        await manager.disconnect(str(user.id))


# ============ AVATAR ROUTES ============

@app.get("/api/avatars")
async def get_available_avatars():
    """get list of available avatars"""
    try:
        if os.path.exists(settings.avatars_config_path):
            with open(settings.avatars_config_path, 'r') as f:
                data = json.load(f)
                return {"avatars": data.get("avatars", [])}
    except Exception as e:
        logger.error(f"Error loading avatars: {e}")
    
    return {
        "avatars": [
            {"id": "default", "name": "Default", "url": "/avatars/default.png"},
        ]
    }


# ============ UTILITY ROUTES ============

@app.get("/api/stats")
async def get_stats(db: AsyncSession = Depends(get_db)):
    """get application statistics"""
    msg_count = await db.execute(select(func.count(Message.id)))
    reaction_count = await db.execute(select(func.count(Reaction.id)))
    
    return {
        "messages": msg_count.scalar(),
        "reactions": reaction_count.scalar(),
        "online": manager.get_online_count()
    }


@app.get("/api/health")
async def health_check():
    """health check endpoint"""
    return {"status": "healthy", "timestamp": datetime.now(timezone.utc).isoformat()}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        "main:app", 
        host="0.0.0.0", 
        port=8000,
        reload=settings.debug
    )