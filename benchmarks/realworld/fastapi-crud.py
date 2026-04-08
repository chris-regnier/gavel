# Copyright 2018 Sebastián Ramírez. All rights reserved.
# Licensed under the MIT License.
# Source: github.com/tiangolo/fastapi (MIT License)
# This is a representative snippet for benchmarking purposes.

from __future__ import annotations

from datetime import datetime
from typing import List, Optional

from fastapi import Depends, FastAPI, HTTPException, Query, status
from fastapi.security import OAuth2PasswordBearer
from pydantic import BaseModel, EmailStr, Field
from sqlalchemy.orm import Session

app = FastAPI(title="User Service", version="1.0.0")
oauth2_scheme = OAuth2PasswordBearer(tokenUrl="token")


# ---------------------------------------------------------------------------
# Pydantic schemas
# ---------------------------------------------------------------------------

class UserBase(BaseModel):
    name: str = Field(..., min_length=1, max_length=100)
    email: EmailStr
    role: str = Field("user", pattern="^(admin|user|viewer)$")


class UserCreate(UserBase):
    password: str = Field(..., min_length=8)


class UserUpdate(BaseModel):
    name: Optional[str] = Field(None, min_length=1, max_length=100)
    email: Optional[EmailStr] = None
    role: Optional[str] = Field(None, pattern="^(admin|user|viewer)$")


class UserResponse(UserBase):
    id: int
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True


class PaginatedUsers(BaseModel):
    items: List[UserResponse]
    total: int
    limit: int
    offset: int


# ---------------------------------------------------------------------------
# Dependency stubs (replace with real DB session factory)
# ---------------------------------------------------------------------------

def get_db():
    """Yield a database session."""
    # In production, yield from a sessionmaker and close in finally block.
    raise NotImplementedError("wire up your DB session here")


def get_current_user(
    token: str = Depends(oauth2_scheme),
    db: Session = Depends(get_db),
):
    """Decode JWT and return the authenticated user."""
    from jose import JWTError, jwt  # type: ignore

    try:
        payload = jwt.decode(token, "secret-key", algorithms=["HS256"])
        user_id: int = int(payload.get("sub"))
    except (JWTError, TypeError, ValueError):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Could not validate credentials",
            headers={"WWW-Authenticate": "Bearer"},
        )

    user = db.query(UserORM).filter(UserORM.id == user_id).first()
    if user is None:
        raise HTTPException(status_code=404, detail="User not found")
    return user


# ---------------------------------------------------------------------------
# ORM model stub
# ---------------------------------------------------------------------------

class UserORM:
    """Placeholder ORM model. Replace with SQLAlchemy declarative base."""

    id: int
    name: str
    email: str
    role: str
    hashed_password: str
    created_at: datetime
    updated_at: datetime


# ---------------------------------------------------------------------------
# CRUD endpoints
# ---------------------------------------------------------------------------

@app.get("/users", response_model=PaginatedUsers, tags=["users"])
def list_users(
    limit: int = Query(20, ge=1, le=100),
    offset: int = Query(0, ge=0),
    db: Session = Depends(get_db),
    _current_user: UserORM = Depends(get_current_user),
):
    """Return a paginated list of users."""
    query = db.query(UserORM)
    total = query.count()
    items = query.offset(offset).limit(limit).all()
    return PaginatedUsers(items=items, total=total, limit=limit, offset=offset)


@app.get("/users/{user_id}", response_model=UserResponse, tags=["users"])
def get_user(
    user_id: int,
    db: Session = Depends(get_db),
    _current_user: UserORM = Depends(get_current_user),
):
    """Fetch a single user by ID."""
    user = db.query(UserORM).filter(UserORM.id == user_id).first()
    if user is None:
        raise HTTPException(status_code=404, detail="User not found")
    return user


@app.post("/users", response_model=UserResponse, status_code=status.HTTP_201_CREATED, tags=["users"])
def create_user(
    payload: UserCreate,
    db: Session = Depends(get_db),
    _current_user: UserORM = Depends(get_current_user),
):
    """Create a new user. Requires admin role."""
    if _current_user.role != "admin":
        raise HTTPException(status_code=403, detail="Admin role required")

    existing = db.query(UserORM).filter(UserORM.email == payload.email).first()
    if existing:
        raise HTTPException(status_code=409, detail="Email already registered")

    from passlib.context import CryptContext  # type: ignore

    pwd_ctx = CryptContext(schemes=["bcrypt"], deprecated="auto")
    now = datetime.utcnow()
    user = UserORM()
    user.name = payload.name
    user.email = payload.email
    user.role = payload.role
    user.hashed_password = pwd_ctx.hash(payload.password)
    user.created_at = now
    user.updated_at = now

    db.add(user)
    db.commit()
    db.refresh(user)
    return user


@app.patch("/users/{user_id}", response_model=UserResponse, tags=["users"])
def update_user(
    user_id: int,
    payload: UserUpdate,
    db: Session = Depends(get_db),
    current_user: UserORM = Depends(get_current_user),
):
    """Update user fields. Users can update their own profile; admins can update any."""
    user = db.query(UserORM).filter(UserORM.id == user_id).first()
    if user is None:
        raise HTTPException(status_code=404, detail="User not found")

    if current_user.id != user_id and current_user.role != "admin":
        raise HTTPException(status_code=403, detail="Insufficient permissions")

    update_data = payload.dict(exclude_unset=True)
    for field, value in update_data.items():
        setattr(user, field, value)
    user.updated_at = datetime.utcnow()

    db.commit()
    db.refresh(user)
    return user


@app.delete("/users/{user_id}", status_code=status.HTTP_204_NO_CONTENT, tags=["users"])
def delete_user(
    user_id: int,
    db: Session = Depends(get_db),
    current_user: UserORM = Depends(get_current_user),
):
    """Delete a user. Requires admin role or self-deletion."""
    user = db.query(UserORM).filter(UserORM.id == user_id).first()
    if user is None:
        raise HTTPException(status_code=404, detail="User not found")

    if current_user.id != user_id and current_user.role != "admin":
        raise HTTPException(status_code=403, detail="Insufficient permissions")

    db.delete(user)
    db.commit()
