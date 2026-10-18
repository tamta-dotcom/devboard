package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	DB        *pgxpool.Pool
	JWTSecret []byte
	TokenTTL  time.Duration
}

type credentials struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type user struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type claims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON request")
		return false
	}

	return true
}

func validEmail(email string) bool {
	if len(email) > 254 || strings.TrimSpace(email) != email {
		return false
	}

	address, err := mail.ParseAddress(email)
	return err == nil && address.Address == email &&
		strings.Contains(email, "@")
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))

	if input.Name == "" || utf8.RuneCountInString(input.Name) > 100 {
		writeError(w, http.StatusBadRequest, "Name must be between 1 and 100 characters")
		return
	}

	if !validEmail(input.Email) {
		writeError(w, http.StatusBadRequest, "A valid email is required")
		return
	}

	if len(input.Password) < 12 || len(input.Password) > 72 {
		writeError(w, http.StatusBadRequest, "Password must be between 12 and 72 bytes")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(input.Password), bcrypt.DefaultCost,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create account")
		return
	}

	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create account")
		return
	}

	// Set UUID version 4 and RFC 4122 variant bits.
	idBytes[6] = (idBytes[6] & 0x0f) | 0x40
	idBytes[8] = (idBytes[8] & 0x3f) | 0x80

	id := hex.EncodeToString(idBytes)
	id = id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" +
		id[16:20] + "-" + id[20:]

	var created user
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO users (id, name, email, password_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, email, created_at
	`, id, input.Name, input.Email, string(passwordHash)).Scan(
		&created.ID, &created.Name, &created.Email, &created.CreatedAt,
	)

	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			writeError(w, http.StatusConflict, "An account with this email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not create account")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"user": created})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}

	email := strings.ToLower(strings.TrimSpace(input.Email))

	var found user
	var passwordHash string

	err := h.DB.QueryRow(r.Context(), `
		SELECT id, name, email, password_hash, created_at
		FROM users WHERE email = $1
	`, email).Scan(
		&found.ID, &found.Name, &found.Email,
		&passwordHash, &found.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "Invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "Login failed")
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}

	now := time.Now()
	tokenClaims := claims{
		Email: found.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   found.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(h.TokenTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims)
	signedToken, err := token.SignedString(h.JWTSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": signedToken,
		"token_type":   "Bearer",
		"expires_in":   int(h.TokenTTL.Seconds()),
		"user":         found,
	})
}

// RequireAuth validates a Bearer token before allowing access.
func (h *AuthHandler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		rawToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		parsed, err := jwt.ParseWithClaims(rawToken, &claims{}, func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return h.JWTSecret, nil
		}, jwt.WithValidMethods([]string{"HS256"}))

		if err != nil || !parsed.Valid {
			writeError(w, http.StatusUnauthorized, "Invalid or expired token")
			return
		}

		tokenClaims, ok := parsed.Claims.(*claims)
		if !ok || tokenClaims.Subject == "" {
			writeError(w, http.StatusUnauthorized, "Invalid token claims")
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey{}, tokenClaims.Subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type userIDKey struct{}

func CurrentUserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey{}).(string)
	return id, ok
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	id, ok := CurrentUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var found user
	err := h.DB.QueryRow(r.Context(), `
		SELECT id, name, email, created_at FROM users WHERE id = $1
	`, id).Scan(&found.ID, &found.Name, &found.Email, &found.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "User no longer exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"user": found})
}
