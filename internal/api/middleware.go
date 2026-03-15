package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/john/botsapp/internal/auth"
	"github.com/john/botsapp/internal/logger"
)

// DualAuthMiddleware accepts either a JWT Bearer token or an X-Service-Token header.
// If X-Service-Token is valid, it extracts sender_user_id from the request body
// and injects synthetic claims into the context. The body is rewound so downstream
// handlers can read it normally.
func DualAuthMiddleware(jwtSvc *auth.JWTService, serviceToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try service token first
			if svcToken := r.Header.Get("X-Service-Token"); svcToken != "" && serviceToken != "" && subtle.ConstantTimeCompare([]byte(svcToken), []byte(serviceToken)) == 1 {
				// Buffer the body so we can peek and then rewind
				bodyBytes, err := io.ReadAll(r.Body)
				r.Body.Close()
				if err != nil {
					http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
					return
				}

				var peek struct {
					SenderUserID int64 `json:"sender_user_id,string"`
				}
				if err := json.Unmarshal(bodyBytes, &peek); err != nil || peek.SenderUserID == 0 {
					http.Error(w, `{"error":"sender_user_id required in body for service token auth"}`, http.StatusBadRequest)
					return
				}

				// Rewind the body for the downstream handler
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

				// Inject synthetic claims with the sender's user ID
				claims := &auth.Claims{
					UserID: peek.SenderUserID,
				}
				ctx := context.WithValue(r.Context(), auth.ClaimsKey, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Fall back to JWT middleware
			jwtSvc.Middleware(next).ServeHTTP(w, r)
		})
	}
}

// RequestLogger is an HTTP middleware that logs incoming requests and their completion status
// using the global structured logger.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		reqID := middleware.GetReqID(r.Context())

		logger.Info("Incoming request", map[string]interface{}{
			"method":      r.Method,
			"path":        r.URL.Path,
			"remote_addr": r.RemoteAddr,
			"request_id":  reqID,
		})

		defer func() {
			logger.Info("Request completed", map[string]interface{}{
				"method":        r.Method,
				"path":          r.URL.Path,
				"status":        ww.Status(),
				"bytes_written": ww.BytesWritten(),
				"duration_ms":   time.Since(start).Milliseconds(),
				"request_id":    reqID,
			})
		}()

		next.ServeHTTP(ww, r)
	})
}
