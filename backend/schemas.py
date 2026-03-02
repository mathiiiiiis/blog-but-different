from pydantic import BaseModel, EmailStr, Field
from typing import Optional, List
from datetime import datetime


class LoginRequest(BaseModel):
    email: EmailStr
    password: str


class PasswordChangeRequest(BaseModel):
    new_password: str = Field(..., min_length=8)


class TokenResponse(BaseModel):
    access_token: str
    token_type: str = "bearer"
    must_change_password: bool = False


class UserBase(BaseModel):
    username: str
    avatar: str = "default"


class UserCreate(UserBase):
    pass


class GuestCreate(BaseModel):
    avatar: str = "default"


class UserResponse(BaseModel):
    id: str
    username: str
    is_admin: bool
    can_post: bool = False
    avatar: str
    created_at: datetime
    
    class Config:
        from_attributes = True


class Attachment(BaseModel):
    type: str  #"image", "audio", "video", "file", "gif"
    url: str
    name: str
    size: Optional[int] = None
    object_name: Optional[str] = None
    gif_id: Optional[str] = None
    preview_url: Optional[str] = None


class ReactionBase(BaseModel):
    emoji: str = Field(..., max_length=50)
    custom_emoji_id: Optional[str] = None


class ReactionCreate(ReactionBase):
    pass


class ReactionResponse(BaseModel):
    id: str
    emoji: str
    user_id: str
    username: str
    created_at: datetime
    
    class Config:
        from_attributes = True


class ReactionSummary(BaseModel):
    emoji: str
    count: int
    users: List[str]
    user_avatars: Optional[List[str]] = None
    custom_emoji_url: Optional[str] = None


class MessageBase(BaseModel):
    content: Optional[str] = None


class MessageCreate(MessageBase):
    pass


class MessageReplyInfo(BaseModel):
    id: str
    content: Optional[str]
    author_username: str
    
    class Config:
        from_attributes = True


class MessageResponse(BaseModel):
    id: str
    content: Optional[str]
    author_id: str
    author_username: str
    author_avatar: str
    is_admin: bool
    is_pinned: bool = False
    attachments: List[Attachment] = []
    reactions: List[ReactionSummary] = []
    created_at: datetime
    updated_at: Optional[datetime] = None
    reply_to: Optional[MessageReplyInfo] = None
    
    class Config:
        from_attributes = True


class MessageList(BaseModel):
    messages: List[MessageResponse]
    pinned_messages: List[MessageResponse] = []
    total: int
    has_more: bool


class CommandResponse(BaseModel):
    success: bool
    command: str
    message: str
    data: Optional[dict] = None


class WSEvent(BaseModel):
    type: str  #"message", "reaction_add", "reaction_remove", "typing", "user_join", "user_leave"
    data: dict


class CustomEmojiResponse(BaseModel):
    id: str
    name: str
    url: str
    created_at: datetime
    
    class Config:
        from_attributes = True


class GifSearchResult(BaseModel):
    id: str
    title: str
    url: str
    preview_url: str
    width: int
    height: int


class GifSearchResponse(BaseModel):
    results: List[GifSearchResult]
    next: Optional[str] = None