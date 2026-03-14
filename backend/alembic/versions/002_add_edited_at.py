"""add edited_at to messages

Revision ID: 002_add_edited_at
Revises: 001_add_emojis
Create Date: 2026-03-14

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy import inspect

revision: str = '002_add_edited_at'
down_revision: Union[str, None] = '001_add_emojis'
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


def upgrade() -> None:
    #on a fresh db == base tables dont exist yet
    #create_all handles the full schema
    if not table_exists('messages'):
        return

    if not column_exists('messages', 'edited_at'):
        op.add_column('messages', sa.Column('edited_at', sa.DateTime(timezone=True), nullable=True))


def downgrade() -> None:
    if column_exists('messages', 'edited_at'):
        op.drop_column('messages', 'edited_at')
