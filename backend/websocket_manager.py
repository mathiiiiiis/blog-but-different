from fastapi import WebSocket
from typing import Dict, List, Optional
import asyncio
import logging
import uuid

logger = logging.getLogger(__name__)


class ConnectionManager:
    def __init__(self):
        #connection_id -> WebSocket
        self.active_connections: Dict[str, WebSocket] = {}
        #user_id -> list of connection_ids
        self.user_connections: Dict[str, List[str]] = {}
        #connection_id -> user_info
        self.connection_info: Dict[str, dict] = {}
        self._lock = asyncio.Lock()
    
    async def connect(self, websocket: WebSocket, user_id: str, user_info: dict) -> str:
        await websocket.accept()
        connection_id = str(uuid.uuid4())
        
        async with self._lock:
            self.active_connections[connection_id] = websocket
            self.connection_info[connection_id] = {
                **user_info,
                "user_id": user_id,
                "connection_id": connection_id
            }
            
            if user_id not in self.user_connections:
                self.user_connections[user_id] = []
            self.user_connections[user_id].append(connection_id)
        
        is_first_connection = len(self.user_connections.get(user_id, [])) == 1
        
        logger.info(f"User {user_info.get('username', user_id)} connected (conn: {connection_id[:8]}). Total connections: {len(self.active_connections)}")
        
        if is_first_connection:
            await self.broadcast({
                "type": "user_join",
                "data": {
                    "user_id": user_id,
                    **user_info,
                    "online_count": self.get_online_count()
                }
            }, exclude_connection=connection_id)
        
        return connection_id
    
    async def disconnect(self, connection_id: str):
        user_info = {}
        user_id = None
        is_last_connection = False
        
        async with self._lock:
            if connection_id in self.connection_info:
                user_info = self.connection_info.pop(connection_id)
                user_id = user_info.get("user_id")
            
            if connection_id in self.active_connections:
                del self.active_connections[connection_id]
            
            if user_id and user_id in self.user_connections:
                if connection_id in self.user_connections[user_id]:
                    self.user_connections[user_id].remove(connection_id)
                if not self.user_connections[user_id]:
                    del self.user_connections[user_id]
                    is_last_connection = True
        
        logger.info(f"User {user_info.get('username', 'unknown')} disconnected (conn: {connection_id[:8]}). Total connections: {len(self.active_connections)}")
        
        if is_last_connection and user_id:
            await self.broadcast({
                "type": "user_leave",
                "data": {
                    "user_id": user_id,
                    **{k: v for k, v in user_info.items() if k not in ["connection_id"]},
                    "online_count": self.get_online_count()
                }
            })
    
    async def send_personal(self, user_id: str, message: dict):
        connection_ids = self.user_connections.get(user_id, [])
        disconnected = []
        
        for conn_id in connection_ids:
            if conn_id in self.active_connections:
                try:
                    await self.active_connections[conn_id].send_json(message)
                except Exception as e:
                    logger.warning(f"Failed to send message to connection {conn_id[:8]}: {e}")
                    disconnected.append(conn_id)
        
        for conn_id in disconnected:
            await self.disconnect(conn_id)
    
    async def broadcast(self, message: dict, exclude_user: Optional[str] = None, exclude_connection: Optional[str] = None):
        disconnected = []
        
        async with self._lock:
            connections = dict(self.active_connections)
            conn_info = dict(self.connection_info)
        
        for conn_id, websocket in connections.items():
            info = conn_info.get(conn_id, {})
            user_id = info.get("user_id")
            
            if exclude_user and user_id == exclude_user:
                continue
            
            if exclude_connection and conn_id == exclude_connection:
                continue
            
            try:
                await websocket.send_json(message)
            except Exception as e:
                logger.warning(f"Failed to broadcast to connection {conn_id[:8]}: {e}")
                disconnected.append(conn_id)
        
        for conn_id in disconnected:
            await self.disconnect(conn_id)
    
    def get_online_users(self) -> list:
        seen_users = set()
        users = []
        
        for conn_id, info in self.connection_info.items():
            user_id = info.get("user_id")
            if user_id and user_id not in seen_users:
                seen_users.add(user_id)
                users.append({
                    "user_id": user_id,
                    "username": info.get("username"),
                    "avatar": info.get("avatar"),
                    "is_admin": info.get("is_admin"),
                })
        
        return users
    
    def get_online_count(self) -> int:
        return len(self.user_connections)
    
    def is_user_online(self, user_id: str) -> bool:
        return user_id in self.user_connections and len(self.user_connections[user_id]) > 0
    
    def get_user_connection_count(self, user_id: str) -> int:
        return len(self.user_connections.get(user_id, []))


manager = ConnectionManager()