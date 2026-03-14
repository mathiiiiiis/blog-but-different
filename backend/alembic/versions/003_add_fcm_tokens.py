"""add fcm_tokens table

Revision ID: 003_add_fcm_tokens
Revises: 002_add_edited_at
Create Date: 2026-03-14

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy import inspect

revision: str = '003_add_fcm_tokens'
down_revision: Union[str, None] = '002_add_edited_at'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def table_exists(table_name: str) -> bool:
    bind = op.get_bind()
    inspector = inspect(bind)
    return table_name in inspector.get_table_names()


def upgrade() -> None:
    if table_exists('fcm_tokens'):
        return
    op.create_table(
        'fcm_tokens',
        sa.Column('id', sa.String(36), primary_key=True),
        sa.Column('token', sa.String(512), nullable=False, unique=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now()),
    )
    op.create_index('ix_fcm_tokens_token', 'fcm_tokens', ['token'], unique=True)


def downgrade() -> None:
    op.drop_index('ix_fcm_tokens_token', table_name='fcm_tokens')
    op.drop_table('fcm_tokens')
