// Copyright (c) 2010 TJ Holowaychuk. All rights reserved.
// Licensed under the MIT License.
// Source: github.com/expressjs/express (MIT License)
// This is a representative snippet for benchmarking purposes.

import { Request, Response, NextFunction, Router } from "express";
import jwt from "jsonwebtoken";
import bcrypt from "bcryptjs";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface AuthUser {
  id: number;
  email: string;
  role: "admin" | "user" | "viewer";
}

export interface AuthenticatedRequest extends Request {
  user?: AuthUser;
}

export interface UserRepository {
  findByEmail(email: string): Promise<AuthUser & { hashedPassword: string } | null>;
  findById(id: number): Promise<AuthUser | null>;
  create(email: string, hashedPassword: string, role: string): Promise<AuthUser>;
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const JWT_SECRET = process.env.JWT_SECRET ?? "";
const JWT_EXPIRES_IN = process.env.JWT_EXPIRES_IN ?? "1h";
const BCRYPT_ROUNDS = 12;

if (!JWT_SECRET) {
  throw new Error("JWT_SECRET environment variable must be set");
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function signToken(user: AuthUser): string {
  return jwt.sign(
    { sub: user.id, email: user.email, role: user.role },
    JWT_SECRET,
    { expiresIn: JWT_EXPIRES_IN }
  );
}

function verifyToken(token: string): AuthUser {
  const payload = jwt.verify(token, JWT_SECRET) as {
    sub: number;
    email: string;
    role: "admin" | "user" | "viewer";
  };
  return { id: payload.sub, email: payload.email, role: payload.role };
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

/**
 * authenticate extracts and verifies the Bearer JWT from the Authorization
 * header. On success it sets req.user. On failure it returns 401.
 */
export function authenticate(
  req: AuthenticatedRequest,
  res: Response,
  next: NextFunction
): void {
  const authHeader = req.headers.authorization;
  if (!authHeader || !authHeader.startsWith("Bearer ")) {
    res.status(401).json({ error: "Authentication required" });
    return;
  }

  const token = authHeader.slice(7);
  try {
    req.user = verifyToken(token);
    next();
  } catch (err) {
    if (err instanceof jwt.TokenExpiredError) {
      res.status(401).json({ error: "Token expired" });
    } else {
      res.status(401).json({ error: "Invalid token" });
    }
  }
}

/**
 * requireRole returns a middleware that allows only users with the specified
 * role. Must be used after authenticate().
 */
export function requireRole(role: "admin" | "user" | "viewer") {
  return (req: AuthenticatedRequest, res: Response, next: NextFunction): void => {
    if (!req.user) {
      res.status(401).json({ error: "Authentication required" });
      return;
    }
    if (req.user.role !== role) {
      res.status(403).json({ error: "Insufficient permissions" });
      return;
    }
    next();
  };
}

// ---------------------------------------------------------------------------
// Auth router factory
// ---------------------------------------------------------------------------

export function createAuthRouter(repo: UserRepository): Router {
  const router = Router();

  /**
   * POST /auth/login
   * Body: { email, password }
   * Returns: { token, user }
   */
  router.post("/login", async (req: Request, res: Response) => {
    const { email, password } = req.body as { email?: string; password?: string };

    if (!email || !password) {
      res.status(400).json({ error: "email and password are required" });
      return;
    }

    const record = await repo.findByEmail(email);
    if (!record) {
      res.status(401).json({ error: "Invalid credentials" });
      return;
    }

    const match = await bcrypt.compare(password, record.hashedPassword);
    if (!match) {
      res.status(401).json({ error: "Invalid credentials" });
      return;
    }

    const user: AuthUser = { id: record.id, email: record.email, role: record.role };
    const token = signToken(user);
    res.json({ token, user });
  });

  /**
   * POST /auth/register
   * Body: { email, password, role? }
   * Returns: { token, user }
   */
  router.post("/register", async (req: Request, res: Response) => {
    const { email, password, role = "user" } = req.body as {
      email?: string;
      password?: string;
      role?: string;
    };

    if (!email || !password) {
      res.status(400).json({ error: "email and password are required" });
      return;
    }

    if (password.length < 8) {
      res.status(400).json({ error: "password must be at least 8 characters" });
      return;
    }

    const existing = await repo.findByEmail(email);
    if (existing) {
      res.status(409).json({ error: "Email already registered" });
      return;
    }

    const hashedPassword = await bcrypt.hash(password, BCRYPT_ROUNDS);
    const user = await repo.create(email, hashedPassword, role);
    const token = signToken(user);
    res.status(201).json({ token, user });
  });

  /**
   * GET /auth/me
   * Returns the authenticated user's profile.
   */
  router.get("/me", authenticate, async (req: AuthenticatedRequest, res: Response) => {
    const user = await repo.findById(req.user!.id);
    if (!user) {
      res.status(404).json({ error: "User not found" });
      return;
    }
    res.json(user);
  });

  return router;
}
