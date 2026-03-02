"""add custom emojis

Revision ID: 001_add_emojis
Revises: 
Create Date: 2026-01-03

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy import inspect

revision: str = '001_add_emojis'
down_revision: Union[str, None] = None
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def column_exists(table_name: str, column_name: str) -> bool:
    bind = op.get_bind()
    inspector = inspect(bind)
    columns = [c['name'] for c in inspector.get_columns(table_name)]
    return column_name in columns


def table_exists(table_name: str) -> bool:
    bind = op.get_bind()
    inspector = inspect(bind)
    return table_name in inspector.get_table_names()


def index_exists(table_name: str, index_name: str) -> bool:
    bind = op.get_bind()
    inspector = inspect(bind)
    indexes = [idx['name'] for idx in inspector.get_indexes(table_name)]
    return index_name in indexes


def upgrade() -> None:
    if not table_exists('custom_emojis'):
        op.create_table(
            'custom_emojis',
            sa.Column('id', sa.String(36), primary_key=True),
            sa.Column('name', sa.String(50), nullable=False, unique=True),
            sa.Column('url', sa.String(500), nullable=False),
            sa.Column('object_name', sa.String(255), nullable=True),
            sa.Column('created_by_id', sa.String(36), sa.ForeignKey('users.id', ondelete='SET NULL'), nullable=True),
            sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now()),
        )
    
    if table_exists('custom_emojis') and not index_exists('custom_emojis', 'ix_custom_emojis_name'):
        op.create_index('ix_custom_emojis_name', 'custom_emojis', ['name'])
    
    if not column_exists('reactions', 'custom_emoji_id'):
        op.add_column('reactions', sa.Column('custom_emoji_id', sa.String(36), nullable=True))
        op.create_foreign_key(
            'fk_reactions_custom_emoji_id',
            'reactions', 'custom_emojis',
            ['custom_emoji_id'], ['id'],
            ondelete='SET NULL'
        )
    
    op.alter_column('reactions', 'emoji', type_=sa.String(50), existing_type=sa.String(10))


def downgrade() -> None:
    if column_exists('reactions', 'custom_emoji_id'):
        op.drop_constraint('fk_reactions_custom_emoji_id', 'reactions', type_='foreignkey')
        op.drop_column('reactions', 'custom_emoji_id')
    
    op.alter_column('reactions', 'emoji', type_=sa.String(10), existing_type=sa.String(50))

    if table_exists('custom_emojis'):
        if index_exists('custom_emojis', 'ix_custom_emojis_name'):
            op.drop_index('ix_custom_emojis_name', 'custom_emojis')
        op.drop_table('custom_emojis')