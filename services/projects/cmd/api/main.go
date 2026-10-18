package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey string

const userIDKey contextKey = "userID"

type Project struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type App struct {
	db        *pgxpool.Pool
	jwtSecret []byte
}

type createProjectRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type updateProjectRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON request")
		return false
	}

	return true
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	s := hex.EncodeToString(b)
	return s[:8] + "-" + s[8:12] + "-" +
		s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}

var uuidPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

func (a *App) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			respondError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		rawToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		token, err := jwt.ParseWithClaims(
			rawToken,
			&jwt.RegisteredClaims{},
			func(token *jwt.Token) (any, error) {
				if token.Method != jwt.SigningMethodHS256 {
					return nil, errors.New("unexpected signing method")
				}
				return a.jwtSecret, nil
			},
			jwt.WithValidMethods([]string{"HS256"}),
		)

		if err != nil || token == nil || !token.Valid {
			respondError(w, http.StatusUnauthorized, "Invalid or expired token")
			return
		}

		claims, ok := token.Claims.(*jwt.RegisteredClaims)
		if !ok || claims.Subject == "" {
			respondError(w, http.StatusUnauthorized, "Invalid token claims")
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, claims.Subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ownerID(r *http.Request) string {
	id, _ := r.Context().Value(userIDKey).(string)
	return id
}

func (a *App) listProjects(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `
		SELECT id, title, description, status, created_at, updated_at
		FROM projects.projects
		WHERE owner_id = $1
		ORDER BY created_at DESC
	`, ownerID(r))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Could not load projects")
		return
	}
	defer rows.Close()

	projects := make([]Project, 0)
	for rows.Next() {
		var p Project
		if err := rows.Scan(
			&p.ID, &p.Title, &p.Description, &p.Status,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			respondError(w, http.StatusInternalServerError, "Could not read projects")
			return
		}
		projects = append(projects, p)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "Could not read projects")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (a *App) createProject(w http.ResponseWriter, r *http.Request) {
	var input createProjectRequest
	if !decodeJSON(w, r, &input) {
		return
	}

	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)

	if input.Title == "" || len(input.Title) > 120 {
		respondError(w, http.StatusBadRequest, "Title must be between 1 and 120 bytes")
		return
	}
	if len(input.Description) > 5000 {
		respondError(w, http.StatusBadRequest, "Description is too long")
		return
	}

	id, err := newUUID()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Could not create project")
		return
	}

	var p Project
	err = a.db.QueryRow(r.Context(), `
		INSERT INTO projects.projects
			(id, owner_id, title, description, status)
		VALUES ($1, $2, $3, $4, 'active')
		RETURNING id, title, description, status, created_at, updated_at
	`, id, ownerID(r), input.Title, input.Description).Scan(
		&p.ID, &p.Title, &p.Description, &p.Status,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Could not create project")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{"project": p})
}

func (a *App) getProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		respondError(w, http.StatusBadRequest, "Invalid project ID")
		return
	}

	var p Project
	err := a.db.QueryRow(r.Context(), `
		SELECT id, title, description, status, created_at, updated_at
		FROM projects.projects
		WHERE id = $1 AND owner_id = $2
	`, id, ownerID(r)).Scan(
		&p.ID, &p.Title, &p.Description, &p.Status,
		&p.CreatedAt, &p.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		respondError(w, http.StatusNotFound, "Project not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Could not load project")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"project": p})
}

func (a *App) updateProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		respondError(w, http.StatusBadRequest, "Invalid project ID")
		return
	}

	var input updateProjectRequest
	if !decodeJSON(w, r, &input) {
		return
	}

	if input.Title == nil && input.Description == nil && input.Status == nil {
		respondError(w, http.StatusBadRequest, "Provide at least one field to update")
		return
	}

	if input.Title != nil {
		*input.Title = strings.TrimSpace(*input.Title)
		if *input.Title == "" || len(*input.Title) > 120 {
			respondError(w, http.StatusBadRequest, "Invalid project title")
			return
		}
	}
	if input.Description != nil {
		*input.Description = strings.TrimSpace(*input.Description)
		if len(*input.Description) > 5000 {
			respondError(w, http.StatusBadRequest, "Description is too long")
			return
		}
	}
	if input.Status != nil && *input.Status != "active" && *input.Status != "archived" {
		respondError(w, http.StatusBadRequest, "Status must be active or archived")
		return
	}

	var p Project
	err := a.db.QueryRow(r.Context(), `
		UPDATE projects.projects
		SET title = COALESCE($1, title),
		    description = COALESCE($2, description),
		    status = COALESCE($3, status),
		    updated_at = NOW()
		WHERE id = $4 AND owner_id = $5
		RETURNING id, title, description, status, created_at, updated_at
	`,
		input.Title, input.Description, input.Status, id, ownerID(r),
	).Scan(
		&p.ID, &p.Title, &p.Description, &p.Status,
		&p.CreatedAt, &p.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		respondError(w, http.StatusNotFound, "Project not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Could not update project")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"project": p})
}

func (a *App) deleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		respondError(w, http.StatusBadRequest, "Invalid project ID")
		return
	}

	result, err := a.db.Exec(r.Context(), `
		DELETE FROM projects.projects
		WHERE id = $1 AND owner_id = $2
	`, id, ownerID(r))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Could not delete project")
		return
	}

	if result.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "Project not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	jwtSecret := os.Getenv("JWT_SECRET")
	port := os.Getenv("PORT")

	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters")
	}
	if port == "" {
		port = "8082"
	}

	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	dbConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatal(err)
	}

	db, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.Ping(pingCtx); err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS projects;

		CREATE TABLE IF NOT EXISTS projects.projects (
			id UUID PRIMARY KEY,
			owner_id UUID NOT NULL,
			title VARCHAR(120) NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status VARCHAR(20) NOT NULL DEFAULT 'active'
				CHECK (status IN ('active', 'archived')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_projects_owner_created
		ON projects.projects (owner_id, created_at DESC);
	`)
	if err != nil {
		log.Fatal(fmt.Errorf("initialize projects schema: %w", err))
	}

	app := &App{db: db, jwtSecret: []byte(jwtSecret)}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{
			"status": "ok", "service": "projects",
		})
	})

	mux.Handle("GET /api/v1/projects",
		app.authenticate(http.HandlerFunc(app.listProjects)))
	mux.Handle("POST /api/v1/projects",
		app.authenticate(http.HandlerFunc(app.createProject)))
	mux.Handle("GET /api/v1/projects/{id}",
		app.authenticate(http.HandlerFunc(app.getProject)))
	mux.Handle("PATCH /api/v1/projects/{id}",
		app.authenticate(http.HandlerFunc(app.updateProject)))
	mux.Handle("DELETE /api/v1/projects/{id}",
		app.authenticate(http.HandlerFunc(app.deleteProject)))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:3000" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}

		if r.Method == http.MethodOptions {
			if origin != "http://localhost:3000" {
				http.Error(w, "Origin not allowed", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		mux.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(), 10*time.Second,
		)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	log.Printf("Projects service listening on :%s", port)
	if err := server.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
