from sqlalchemy import String, Boolean, DateTime, Text, ForeignKey, JSON, Index, Enum as SQLEnum
from sqlalchemy.orm import relationship, declarative_base, Mapped, mapped_column
from sqlalchemy.sql import func
import uuid
from datetime import datetime
from typing import List, Optional
import enum

Base = declarative_base()


def generate_uuid() -> str:
    return str(uuid.uuid4())


class User(Base):
    __tablename__ = "users"
    
    id: Mapped[str] = mapped_column(
        String(36), 
        primary_key=True, 
        default=generate_uuid
    )
    email: Mapped[Optional[str]] = mapped_column(
        String(255), 
        unique=True, 
        nullable=True,
        index=True
    )
    username: Mapped[str] = mapped_column(
        String(50), 
        unique=True, 
        nullable=False,
        index=True
    )
    password_hash: Mapped[Optional[str]] = mapped_column(
        String(255), 
        nullable=True
    )
    is_admin: Mapped[bool] = mapped_column(Boolean, default=False, index=True)
    must_change_password: Mapped[bool] = mapped_column(Boolean, default=False)
    avatar: Mapped[str] = mapped_column(String(50), default="default")
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), 
        server_default=func.now()
    )
    last_seen: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), 
        server_default=func.now(),
        onupdate=func.now()
    )
    
    #relationships
    messages: Mapped[List["Message"]] = relationship(
        "Message", 
        back_populates="author", 
        cascade="all, delete-orphan"
    )
    reactions: Mapped[List["Reaction"]] = relationship(
        "Reaction", 
        back_populates="user", 
        cascade="all, delete-orphan"
    )
    
    @property
    def can_post(self) -> bool:
        """check if user has permission to post messages."""
        return self.is_admin

class CustomEmoji(Base):
    __tablename__ = "custom_emojis"
    
    id: Mapped[str] = mapped_column(
        String(36), 
        primary_key=True, 
        default=generate_uuid
    )
    name: Mapped[str] = mapped_column(
        String(50), 
        unique=True, 
        nullable=False,
        index=True
    )
    url: Mapped[str] = mapped_column(
        String(500), 
        nullable=False
    )
    object_name: Mapped[Optional[str]] = mapped_column(
        String(255),
        nullable=True
    )
    created_by_id: Mapped[Optional[str]] = mapped_column(
        String(36), 
        ForeignKey("users.id", ondelete="SET NULL"), 
        nullable=True
    )
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), 
        server_default=func.now()
    )
    
    #relationships
    created_by: Mapped[Optional["User"]] = relationship("User")


class Message(Base):
    __tablename__ = "messages"
    
    id: Mapped[str] = mapped_column(
        String(36), 
        primary_key=True, 
        default=generate_uuid
    )
    content: Mapped[Optional[str]] = mapped_column(
        Text, 
        nullable=True
    )  #null if only attachments
    author_id: Mapped[str] = mapped_column(
        String(36), 
        ForeignKey("users.id", ondelete="CASCADE"), 
        nullable=False,
        index=True
    )
    reply_to_id: Mapped[Optional[str]] = mapped_column(
        String(36), 
        ForeignKey("messages.id", ondelete="SET NULL"), 
        nullable=True,
        index=True
    )
    is_pinned: Mapped[bool] = mapped_column(
        Boolean, 
        default=False, 
        nullable=False,
        index=True
    )
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), 
        server_default=func.now(),
        index=True
    )
    updated_at: Mapped[Optional[datetime]] = mapped_column(
        DateTime(timezone=True), 
        onupdate=func.now(), 
        nullable=True
    )
    attachments: Mapped[list] = mapped_column(
        JSON, 
        default=list,
        nullable=False
    )  #[{type: "image"|"gif"|"audio"|"video"|"file", url: "...", name: "...", gif_id?: "..."}]
    
    #relationships
    author: Mapped["User"] = relationship("User", back_populates="messages")
    parent: Mapped[Optional["Message"]] = relationship(
        "Message", 
        remote_side=[id], 
        backref="replies"
    )
    reactions: Mapped[List["Reaction"]] = relationship(
        "Reaction", 
        back_populates="message", 
        cascade="all, delete-orphan"
    )
    
    #indexes
    __table_args__ = (
        Index('ix_messages_created_at_desc', created_at.desc()),
        Index('ix_messages_pinned_created', is_pinned, created_at.desc()),
    )


class Reaction(Base):
    __tablename__ = "reactions"
    
    id: Mapped[str] = mapped_column(
        String(36), 
        primary_key=True, 
        default=generate_uuid
    )
    message_id: Mapped[str] = mapped_column(
        String(36), 
        ForeignKey("messages.id", ondelete="CASCADE"), 
        nullable=False,
        index=True
    )
    user_id: Mapped[str] = mapped_column(
        String(36), 
        ForeignKey("users.id", ondelete="CASCADE"), 
        nullable=False,
        index=True
    )
    emoji: Mapped[str] = mapped_column(
        String(50),
        nullable=False
    )
    #for custom emoji reactions
    custom_emoji_id: Mapped[Optional[str]] = mapped_column(
        String(36),
        ForeignKey("custom_emojis.id", ondelete="SET NULL"),
        nullable=True
    )
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), 
        server_default=func.now()
    )
    
    #relationships
    message: Mapped["Message"] = relationship("Message", back_populates="reactions")
    user: Mapped["User"] = relationship("User", back_populates="reactions")
    custom_emoji: Mapped[Optional["CustomEmoji"]] = relationship("CustomEmoji")

    __table_args__ = (
        Index('ix_reactions_message_user_emoji', message_id, user_id, emoji, unique=True),
    )