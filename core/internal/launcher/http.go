package launcher

import (
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net"
	"net/http"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

const maxRequestBytes = 4 << 10

// AllowedOrigins are the dashboard's dev server addresses; only pages served from there may
// call the launcher from a browser.
var AllowedOrigins = []string{"http://localhost:5173", "http://127.0.0.1:5173"}

// NewHandler serves GET /content, POST /launch and GET /status.
func NewHandler(service *Service, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /content", func(w http.ResponseWriter, _ *http.Request) {
		content, err := ScanContent(service.cfg.Paths)
		if err != nil {
			problem(w, logger, http.StatusInternalServerError, err)
			return
		}
		reply(w, logger, http.StatusOK, content)
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, logger, http.StatusOK, service.Status())
	})
	mux.HandleFunc("POST /launch", func(w http.ResponseWriter, r *http.Request) {
		launch(w, r, service, logger)
	})
	return guard(logger, mux)
}

func launch(w http.ResponseWriter, r *http.Request, service *Service, logger *slog.Logger) {
	if media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil ||
		media != "application/json" {
		problem(w, logger, http.StatusUnsupportedMediaType, errors.New("send application/json"))
		return
	}
	var request contracts.LaunchRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		problem(w, logger, http.StatusUnprocessableEntity, err)
		return
	}
	status, err := service.Launch(request)
	switch {
	case errors.Is(err, ErrInvalid):
		problem(w, logger, http.StatusUnprocessableEntity, err)
	case errors.Is(err, ErrBusy):
		problem(w, logger, http.StatusConflict, err)
	case err != nil:
		problem(w, logger, http.StatusInternalServerError, err)
	default:
		reply(w, logger, http.StatusAccepted, status)
	}
}

// guard answers only requests addressed to the loopback name (no DNS rebinding) and, from a
// browser, only from the dashboard's origins; it answers CORS preflights for them.
func guard(logger *slog.Logger, next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range AllowedOrigins {
		allowed[origin] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			problem(w, logger, http.StatusForbidden, errors.New("the launcher answers 127.0.0.1 only"))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if !allowed[origin] {
				problem(w, logger, http.StatusForbidden, errors.New("origin not allowed: "+origin))
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(hostPort string) bool {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func reply(w http.ResponseWriter, logger *slog.Logger, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.Error("reply not written", "error", err)
	}
}

func problem(w http.ResponseWriter, logger *slog.Logger, code int, err error) {
	if code >= http.StatusInternalServerError {
		logger.Error("request failed", "status", code, "error", err)
	}
	reply(w, logger, code, map[string]string{"error": err.Error()})
}
