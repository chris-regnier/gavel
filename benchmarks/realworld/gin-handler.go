//go:build ignore

// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style license.
// Source: github.com/gin-gonic/gin (MIT License)
// This is a representative snippet for benchmarking purposes.

package handlers

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// User represents a user resource.
type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserStore is the interface for user persistence.
type UserStore interface {
	FindByID(id int64) (*User, error)
	FindAll(limit, offset int) ([]*User, error)
	Create(u *User) error
	Update(u *User) error
	Delete(id int64) error
}

// UserHandler groups user-related HTTP handlers.
type UserHandler struct {
	store  UserStore
	logger *log.Logger
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(store UserStore, logger *log.Logger) *UserHandler {
	return &UserHandler{store: store, logger: logger}
}

// RegisterRoutes attaches the handler routes to the given router group.
func (h *UserHandler) RegisterRoutes(rg *gin.RouterGroup) {
	users := rg.Group("/users")
	users.GET("", h.ListUsers)
	users.GET("/:id", h.GetUser)
	users.POST("", h.CreateUser)
	users.PUT("/:id", h.UpdateUser)
	users.DELETE("/:id", h.DeleteUser)
}

// ListUsers handles GET /users with optional pagination.
func (h *UserHandler) ListUsers(c *gin.Context) {
	limit := 20
	offset := 0

	if l := c.Query("limit"); l != "" {
		v, err := strconv.Atoi(l)
		if err != nil || v < 1 || v > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		limit = v
	}

	if o := c.Query("offset"); o != "" {
		v, err := strconv.Atoi(o)
		if err != nil || v < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset"})
			return
		}
		offset = v
	}

	users, err := h.store.FindAll(limit, offset)
	if err != nil {
		h.logger.Printf("FindAll error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users":  users,
		"limit":  limit,
		"offset": offset,
	})
}

// GetUser handles GET /users/:id.
func (h *UserHandler) GetUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	user, err := h.store.FindByID(id)
	if err != nil {
		h.logger.Printf("FindByID(%d) error: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// CreateUserRequest is the request body for creating a user.
type CreateUserRequest struct {
	Name  string `json:"name"  binding:"required,min=1,max=100"`
	Email string `json:"email" binding:"required,email"`
	Role  string `json:"role"  binding:"required,oneof=admin user viewer"`
}

// CreateUser handles POST /users.
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC()
	user := &User{
		Name:      req.Name,
		Email:     req.Email,
		Role:      req.Role,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.store.Create(user); err != nil {
		h.logger.Printf("Create user error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusCreated, user)
}

// UpdateUserRequest is the request body for updating a user.
type UpdateUserRequest struct {
	Name  string `json:"name"  binding:"omitempty,min=1,max=100"`
	Email string `json:"email" binding:"omitempty,email"`
	Role  string `json:"role"  binding:"omitempty,oneof=admin user viewer"`
}

// UpdateUser handles PUT /users/:id.
func (h *UserHandler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	existing, err := h.store.FindByID(id)
	if err != nil {
		h.logger.Printf("FindByID(%d) error: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Email != "" {
		existing.Email = req.Email
	}
	if req.Role != "" {
		existing.Role = req.Role
	}
	existing.UpdatedAt = time.Now().UTC()

	if err := h.store.Update(existing); err != nil {
		h.logger.Printf("Update user(%d) error: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, existing)
}

// DeleteUser handles DELETE /users/:id.
func (h *UserHandler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	existing, err := h.store.FindByID(id)
	if err != nil {
		h.logger.Printf("FindByID(%d) error: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := h.store.Delete(id); err != nil {
		h.logger.Printf("Delete user(%d) error: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.Status(http.StatusNoContent)
}

// RequestLogger returns a gin middleware that logs request details.
func RequestLogger(logger *log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		logger.Printf("%s %s %d %v", c.Request.Method, path, status, latency)
	}
}

// RequireRole returns a middleware that restricts access to users with a given role.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("user_role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if userRole != role {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		c.Next()
	}
}
